use crate::Directory;
use crate::PathComponent;
use crate::Query;
use std::cmp::Ordering;
use std::collections::BinaryHeap;
use std::convert::TryFrom;

/// Internal matcher state. Corresponds to a single character in the query.
#[derive(Clone, Copy)]
struct State {
    /// Total score from matching all characters up to the current one.
    score: u32,
    /// Longest sequence of consecutively-matching query characters in the
    /// target string ending at this character's match.
    consecutive: Consecutive,
}

/// Represents a run of consecutive matching characters in the half-open range
/// [start, end). Indexes are into the target path string, not the query.
#[derive(Clone, Copy)]
struct Consecutive {
    start: usize,
    end: usize,
}

impl Consecutive {
    /// Returns the number of characters covered by the [Consecutive] run.
    fn span(&self) -> u32 {
        u32::try_from(self.end - self.start).unwrap()
    }
}

impl std::fmt::Debug for State {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_fmt(format_args!(
            "[{:3}:{:2}:{:2}]",
            self.score, self.consecutive.start, self.consecutive.end,
        ))
    }
}

/// Represents a filesystem path that matches the query and its associated
/// score. Ordering is reversed, so implements min-heap behavior when used with
/// BinaryHeap.
#[derive(Debug, Eq, PartialEq)]
pub struct Match {
    pub score: u32,
    pub path: String,
}

impl PartialOrd for Match {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.score.partial_cmp(&other.score).unwrap().reverse())
    }
}

impl Ord for Match {
    fn cmp(&self, other: &Self) -> Ordering {
        self.score.cmp(&other.score).reverse()
    }
}

/// Represents a query and the internal state of the fuzzy-find algorithm.
///
/// [Matcher] implements the VS Code fuzzy-find algorithm from
/// https://github.com/microsoft/vscode/blob/main/src/vs/base/common/fuzzyScorer.ts
pub struct Matcher<'a> {
    /// The search query
    query: &'a Query,
    /// A vector of states, one per character in the query, representing the
    /// partial scores after ingesting the given path components.
    states: Vec<State>,
    /// The number of characters in the given path components.
    length: usize,
    /// The maximum number of results to include.
    max_results: usize,
}

impl Matcher<'_> {
    /// Creates a new Matcher.
    pub fn new(query: &Query, max_results: usize) -> Matcher {
        let states = vec![
            State {
                score: 0,
                consecutive: Consecutive { start: 0, end: 0 },
            };
            query.len()
        ];

        if max_results == 0 {
            panic!("max_results must be at least 1");
        }

        Matcher {
            query,
            states,
            length: 0,
            max_results,
        }
    }

    /// Walks a given directory tree, advancing the matcher's state and adding
    /// results to the provided binary heap.
    pub fn walk(&mut self, directory: &Directory, path: &str, h: &mut BinaryHeap<Match>) {
        self.advance(&directory.name);

        let ostates = self.states.to_vec();
        let olength = self.length;
        let path = path.to_owned() + directory.name.as_string();

        for file in &directory.files {
            let score = self.score(file);
            self.states.copy_from_slice(&ostates);
            self.length = olength;

            if score == 0 {
                continue;
            } else if h.len() < self.max_results {
                h.push(Match {
                    score: score,
                    path: path.to_owned() + file.as_string(),
                });
            } else if score > h.peek().unwrap().score {
                h.pop();
                h.push(Match {
                    score: score,
                    path: path.to_owned() + file.as_string(),
                });
            }
        }

        for child in &directory.children {
            if self.query.covered_by(&child.char_set) {
                self.walk(&child, &path, h);
                self.states.copy_from_slice(&ostates);
                self.length = olength;
            }
        }
    }

    /// Advances the matcher with the given path component and returns its
    /// score.
    fn score(&mut self, comp: &PathComponent) -> u32 {
        self.advance(comp);
        return self.states.last().unwrap().score;
    }

    /// Advances the matcher with the given path component.
    fn advance(&mut self, comp: &PathComponent) {
        for (i, item) in comp.iter().enumerate() {
            // Make `i` relative to the start of the path, rather than the start
            // of this path component.
            let i = self.length + i;

            for q in self.query.matches_for_char(item.byte) {
                let mut next = State {
                    score: q.points as u32 + item.bonus as u32,
                    consecutive: Consecutive {
                        start: i,
                        end: i + 1,
                    },
                };

                if q.index > 0 {
                    let prev = self.states[q.index as usize - 1];
                    if prev.score == 0 {
                        continue;
                    }
                    next.score += prev.score;
                    if prev.consecutive.end == i {
                        next.score += prev.consecutive.span() * 5;
                        next.consecutive = Consecutive {
                            start: prev.consecutive.start,
                            end: i + 1,
                        }
                    }
                }

                if next.score > self.states[q.index as usize].score {
                    self.states[q.index as usize] = next;
                }
            }
            // println!("{} {:?}", item.byte, self.states);
        }
        self.length += comp.len();
    }
}

#[cfg(test)]
mod test {
    use super::*;

    #[test]
    fn consecutive_span() {
        let c = Consecutive { start: 0, end: 3 };
        assert_eq!(3, c.span());
    }

    #[test]
    fn match_ord() {
        let m = Match {
            score: 123,
            path: "abc".to_string(),
        };
        let n = Match {
            score: 456,
            path: "pqr".to_string(),
        };
        let o = Match {
            score: 1,
            path: "123".to_string(),
        };

        let mut h = BinaryHeap::new();
        h.push(m);
        h.push(n);
        h.push(o);

        assert_eq!(1, h.pop().unwrap().score);
        assert_eq!(123, h.pop().unwrap().score);
        assert_eq!(456, h.pop().unwrap().score);
    }

    #[test]
    fn advance_score_simple() {
        let q = Query::new("asdf/123.rs").unwrap();
        let mut m = Matcher::new(&q, 100);

        let a = PathComponent::from("abc", true);
        let b = PathComponent::from("SDF", false);
        let c = PathComponent::from("102030.rs", false);
        let x = PathComponent::from("", true);

        m.advance(&a);
        m.advance(&b);
        let score = m.score(&c);

        let chars = 2 + 1 + 1 + 1 + 2 + 2 + 2 + 2 + 2 + 2 + 2;
        let prevs = 8 + 5 + 2 + 2 + 0 + 5 + 0 + 0 + 0 + 4 + 0;
        let concs = 5 + 10 + 15 + 20 + 5 + 10;
        assert_eq!(chars + prevs + concs, score);

        m.advance(&x);
        let score2 = m.score(&x);
        assert_eq!(score, score2);

        let full = PathComponent::from("abc/SDF/102030.rs", true);
        m = Matcher::new(&q, 100);
        let score3 = m.score(&full);
        assert_eq!(score, score3);
    }

    #[test]
    fn advance_score_tail() {
        let q = Query::new("file").unwrap();
        let mut m = Matcher::new(&q, 100);

        let a = PathComponent::from("abc", true);
        let b = PathComponent::from("def", false);
        let c = PathComponent::from("fillet.sh", false);

        m.advance(&a);
        m.advance(&b);
        let score = m.score(&c);

        let chars = 2 + 2 + 2 + 2;
        let prevs = 5 + 0 + 0 + 0;
        let concs = 5 + 10;
        assert_eq!(chars + prevs + concs, score);

        let full = PathComponent::from("abc/def/fillet.sh", true);
        m = Matcher::new(&q, 100);
        let score2 = m.score(&full);
        assert_eq!(score, score2);
    }

    #[test]
    fn advance_score_more() {
        let p = PathComponent::from("abseil/absl/base/bit_cast_test.cc", true);
        let q = Query::new("abseilabsl.c").unwrap();
        let s = Matcher::new(&q, 100).score(&p);
        assert_eq!(151, s);

        let p = PathComponent::from("abseil/absl/flags/flag.cc", true);
        let q = Query::new("abseilabsl.c").unwrap();
        let s = Matcher::new(&q, 100).score(&p);
        assert_eq!(151, s);

        let p = PathComponent::from("firefox/dom/u2f/U2F.cpp", true);
        let q = Query::new("FFX//U2FCPP").unwrap();
        let s = Matcher::new(&q, 100).score(&p);
        assert_eq!(81, s);

        let p = PathComponent::from("rpi-eeprom/LICENSE", true);
        let q = Query::new("LICENSE").unwrap();
        let s = Matcher::new(&q, 100).score(&p);
        assert_eq!(136, s);

        let p = PathComponent::from("libinput/test/litest-device-synaptics-i2c.c", true);
        let q = Query::new("litsyn-2c").unwrap();
        let s = Matcher::new(&q, 100).score(&p);
        assert_eq!(60, s);

        let p = PathComponent::from("libjpeg-turbo/CMakeLists.txt", true);
        let q = Query::new("CMakeLists").unwrap();
        let s = Matcher::new(&q, 100).score(&p);
        assert_eq!(254, s);
    }

    #[test]
    fn walk() {
        let f1 = PathComponent::from("aaa", false);
        let f2 = PathComponent::from("bar", false);
        let c1 = PathComponent::from("child", false);
        let child = Directory::new(c1, vec![f1, f2], vec![]);

        let f3 = PathComponent::from("baz", false);
        let r = PathComponent::from("root", true);
        let root = Directory::new(r, vec![f3], vec![child]);

        let mut h = BinaryHeap::new();
        let q = Query::new("child/aaa").unwrap();
        let mut m = Matcher::new(&q, 100);
        m.walk(&root, "", &mut h);

        assert_eq!(4, m.length); // advanced past "root"
        assert_eq!(1, h.len());
        assert_eq!(18 + 10 + 180, h.peek().unwrap().score);
        assert_eq!("root/child/aaa", h.peek().unwrap().path);

        let mut h = BinaryHeap::new();
        let q = Query::new("/a").unwrap();
        let mut m = Matcher::new(&q, 2);
        m.walk(&root, "", &mut h);

        assert_eq!(2, h.len());
        let res0 = h.pop().unwrap();
        let res1 = h.pop().unwrap();
        assert_eq!(4, res0.score);
        assert_eq!("root/baz", res0.path); // earlier paths win tie-breaker

        assert_eq!(9, res1.score);
        assert_eq!("root/child/aaa", res1.path);
    }
}
