use crate::CharSet;
use array_init::array_init;
use std::convert::TryFrom;

/// A search query.
///
/// [Query] represents a pre-processed version of the user's query that can be
/// quickly matched against packages' directory trees.
pub struct Query {
    /// For fast constant-time lookups, we create an array indexed by character.
    /// For example, if the query contains the letter 'Q' (ASCII value 81),
    /// entry 81 will be populated.
    ///
    /// Entries contain a vector with one element per character in the query, so
    /// entry 81 will contain one element for each time the letter 'Q' appears
    /// in the query.
    ///
    /// [Query] is case-insensitive, so both uppercase and lowercase versions of
    /// each character are entered. The point values for these differ as
    /// determined by the fuzzy-find algorithm.
    ///
    /// Each vector is sorted in reverse index order.
    lookup: [Vec<QChar>; 128],

    /// A [CharSet] over the query string.
    char_set: CharSet,

    /// The original length of the query string, in characters.
    len: usize,
}

/// A character in a [Query].
///
/// Each [QChar] in the [Query]'s `lookup` corresponds to a character in the
/// original query string.
#[derive(Clone, Copy, Debug)]
pub struct QChar {
    /// The index in the original query string where this character appears.
    pub index: u8,
    /// The number of points this character is worth when it's matched. Includes
    /// the 1-point base score and the 1-point bonus for a same-case match.
    pub points: u8,
}

impl Query {
    /// Checks if the query contains the given characters and returns a vector
    /// of matches. Every [QChar] will have an `index` between zero and
    /// `len()-1`.
    pub fn matches_for_char(&self, byte: u8) -> &Vec<QChar> {
        &self.lookup[usize::from(byte)]
    }

    /// Checks if the query contains, at minimum, every character in the given
    /// [CharSet].
    pub fn covered_by(&self, other: &CharSet) -> bool {
        other.covers(&self.char_set)
    }

    /// Returns the length of the query, in characters.
    pub fn len(&self) -> usize {
        self.len
    }

    /// Constructs a [Query] from the given string. If the string contains
    /// non-ASCII characters (character code >= 127) or the null byte, returns
    /// [None].
    pub fn new(query: &str) -> Option<Query> {
        let pairs = query.chars().enumerate().collect::<Vec<(usize, char)>>();
        let mut q = Query {
            lookup: array_init(|_| vec![]),
            char_set: CharSet::new(),
            len: pairs.len(),
        };

        if pairs.len() == 0 {
            return None;
        }

        // Add each character in the query to the data structure. We add items
        // in reverse order so that higher-indexed characters always appear
        // earlier in the `lookup` vectors, which is required by `Matcher`.
        for (i, ch) in pairs.iter().rev() {
            // A case-sensitive match is worth 2 points. If the character is out
            // of range, abort and return None.
            q.add(*i, *ch, 2)?;

            // A match with the opposite case is worth 1 point.
            if ch.is_ascii_lowercase() {
                let upper = ch.to_ascii_uppercase();
                q.add(*i, upper, 1).unwrap();
            } else if ch.is_ascii_uppercase() {
                let lower = ch.to_ascii_lowercase();
                q.add(*i, lower, 1).unwrap();
            }

            // Add to character set
            q.char_set.add(*ch);
        }

        // Compact all vectors for efficiency
        q.lookup.iter_mut().for_each(|v| v.shrink_to_fit());

        Some(q)
    }

    /// Adds a single character to the lookup table. Does *not* perform case
    /// conversion. If the character is out-of-range, returns [None].
    fn add(&mut self, index: usize, ch: char, points: u8) -> Option<()> {
        let u = u32::from(ch);
        if u == 0 || u > 127 {
            return None;
        }
        self.lookup[usize::try_from(u).ok()?].push(QChar {
            index: u8::try_from(index).ok()?,
            points,
        });

        Some(())
    }
}

#[cfg(test)]
mod test {
    use super::*;

    #[test]
    fn new() {
        let query = Query::new("Hi There").unwrap();
        assert_eq!(1, query.lookup['t' as usize].len());
        assert_eq!(1, query.lookup['T' as usize].len());
        assert_eq!(2, query.lookup['e' as usize].len());
        assert_eq!(2, query.lookup['E' as usize].len());
        assert_eq!(1, query.lookup[' ' as usize].len());
        assert_eq!(0, query.lookup['\x00' as usize].len());

        assert_eq!(7, query.lookup['e' as usize][0].index);
        assert_eq!(2, query.lookup['e' as usize][0].points);
        assert_eq!(5, query.lookup['e' as usize][1].index);
        assert_eq!(2, query.lookup['e' as usize][1].points);

        assert_eq!(7, query.lookup['E' as usize][0].index);
        assert_eq!(1, query.lookup['E' as usize][0].points);
        assert_eq!(5, query.lookup['E' as usize][1].index);
        assert_eq!(1, query.lookup['E' as usize][1].points);

        assert_eq!(0x0000002_80640001, query.char_set.extract_internals());

        assert_eq!(8, query.len);

        let query = Query::new("");
        assert!(query.is_none());
    }

    #[test]
    fn invalid() {
        let query = Query::new("abc\x00");
        assert!(query.is_none());
    }

    #[test]
    fn matches_for_char() {
        let query = Query::new("Hi There").unwrap();

        let i = query.matches_for_char('i' as u8);
        assert_eq!(1, i.len());
        assert_eq!(1, i[0].index);
        assert_eq!(2, i[0].points);

        let i2 = query.matches_for_char('I' as u8);
        assert_eq!(1, i2.len());
        assert_eq!(1, i2[0].index);
        assert_eq!(1, i2[0].points);

        let e = query.matches_for_char('e' as u8);
        assert_eq!(2, e.len());
    }

    #[test]
    fn covered_by() {
        let query = Query::new("Hi There").unwrap();
        let mut cs = CharSet::new();
        assert!(!query.covered_by(&cs));

        cs.add('h');
        cs.add('i');
        cs.add('t');
        cs.add('h');
        cs.add('r');
        cs.add(' ');
        assert!(!query.covered_by(&cs));

        cs.add('e');
        assert!(query.covered_by(&cs));

        cs.add('X');
        assert!(query.covered_by(&cs));
    }

    #[test]
    fn len() {
        let q0 = Query::new("Hi There").unwrap();
        assert_eq!(8, q0.len());
    }
}
