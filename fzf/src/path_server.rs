use crate::Directory;
use crate::Matcher;
use crate::Query;
use http::StatusCode;
use reqwest::Url;
use std::collections::BinaryHeap;
use std::collections::HashMap;
use std::time::Instant;

pub struct PathServer {
    commit: String,
    max_results: usize,

    index: HashMap<String, Vec<Directory>>,
}

impl PathServer {
    pub fn new<'a>(commit: String, max_results: usize) -> PathServer {
        PathServer {
            commit,
            max_results,
            index: HashMap::new(),
        }
    }

    pub fn load(&mut self, distro: String, data: &[u8]) {
        let root = Directory::load(data).unwrap();
        self.index.insert(distro, root);
    }

    pub fn handle(&self, url: &Url) -> (StatusCode, String) {
        let parts: Vec<&str> = url.path_segments().unwrap().collect();
        let distro = *parts.get(0).unwrap_or(&"");
        if distro == "" {
            // Request to "/"
            return (StatusCode::OK, format!("Hello from fzf@{}!", self.commit));
        }

        let pkgs = match self.index.get(distro) {
            None => {
                return (StatusCode::NOT_FOUND, "404 Not Found".to_string());
            }
            Some(x) => x,
        };
        if parts.len() > 1 {
            return (StatusCode::NOT_FOUND, "404 Not Found".to_string());
        }

        let params: HashMap<_, _> = url.query_pairs().collect();
        let qstr = params.get("q");
        let query = match qstr.and_then(|q| Query::new(q)) {
            None => {
                return (StatusCode::BAD_REQUEST, "400 Bad Request".to_string());
            }
            Some(x) => x,
        };

        let start = Instant::now();
        let mut h = BinaryHeap::new();
        for root in pkgs {
            Matcher::new(&query, self.max_results).walk(root, "", &mut h);
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

        (StatusCode::OK, body)
    }
}
