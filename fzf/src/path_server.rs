use crate::Directory;
use crate::Matcher;
use crate::Query;
use crate::directory::Arena;
use std::collections::BinaryHeap;
use std::collections::HashMap;
use std::time::Instant;
use url::Url;

pub struct PathServer {
    commit: String,
    max_results: usize,

    arena: Arena,
    index: HashMap<String, Vec<Directory>>,
}

impl PathServer {
    pub fn new(commit: String, max_results: usize) -> PathServer {
        PathServer {
            commit,
            max_results,
            arena: Arena::new(),
            index: HashMap::new(),
        }
    }

    pub fn load(&mut self, distro: String, data: &[u8]) {
        let root = self.arena.load(data).unwrap();
        self.index.insert(distro, root);
    }

    pub fn handle(&self, url: &Url) -> (u16, String) {
        let parts: Vec<&str> = url.path_segments().unwrap().collect();
        let distro = *parts.get(0).unwrap_or(&"");
        if distro == "" {
            // Request to "/"
            return (200, format!("Hello from fzf@{}!\n", self.commit));
        } else if distro == "robots.txt" {
            return (200, format!("User-agent: *\nDisallow: /\n"));
        }

        let pkgs = match self.index.get(distro) {
            None => {
                return (404, "404 Not Found\n".to_string());
            }
            Some(x) => x,
        };
        if parts.len() > 1 {
            return (404, "404 Not Found\n".to_string());
        }

        let params: HashMap<_, _> = url.query_pairs().collect();
        let qstr = params.get("q");
        let query = match qstr.and_then(|q| Query::new(q)) {
            None => {
                return (400, "400 Bad Request\n".to_string());
            }
            Some(x) => x,
        };

        let start = Instant::now();
        let mut h = BinaryHeap::new();
        for root in pkgs {
            Matcher::new(&query, self.max_results, &self.arena).walk(root, "", &mut h, true);
        }

        let mut body = String::new();
        let mut v = h.into_vec();
        v.sort();
        for i in &v {
            body.push_str(&format!("{} {}\n", i.score, i.path));
        }

        body.push_str(&format!("\nQuery: {:?}\n", qstr.unwrap()));
        if v.len() >= self.max_results {
            body.push_str(&format!("Results: {} (truncated)\n", v.len()))
        } else {
            body.push_str(&format!("Results: {}\n", v.len()))
        }
        body.push_str(&format!("Time: {:?}\n", start.elapsed()));

        (200, body)
    }
}
