use http::StatusCode;
use reqwest::Url;
use std::collections::HashMap;
use std::time::Instant;

pub struct TagServer {
    commit: String,
    distro: String,
    buffer: Vec<u8>,
}

impl TagServer {
    pub fn new<'a>(commit: String, distro: String, buffer: Vec<u8>) -> TagServer {
        TagServer {
            commit,
            distro,
            buffer,
        }
    }

    pub fn handle(&self, url: &Url) -> (StatusCode, String) {
        let parts: Vec<&str> = url.path_segments().unwrap().collect();
        let distro = *parts.get(0).unwrap_or(&"");
        if distro == "" {
            // Request to "/"
            return (StatusCode::OK, format!("Hello from ctags@{}!", self.commit));
        }

        if distro != self.distro || parts.len() > 1 {
            return (StatusCode::NOT_FOUND, "404 Not Found".to_string());
        }

        let params: HashMap<_, _> = url.query_pairs().collect();
        let estr = params.get("exact");
        let exact = match estr {
            None => {
                return (StatusCode::BAD_REQUEST, "400 Bad Request".to_string());
            }
            Some(x) => x.to_string(),
        };

        let start = Instant::now();
        let mut body = String::new();

        // Read package table
        let mut data = &self.buffer[..];
        let mut packages = HashMap::new();
        let n_pkgs = rmp::decode::read_array_len(&mut data).unwrap();
        for _ in 0..n_pkgs {
            let (name, tmp) = rmp::decode::read_str_from_slice(data).unwrap();
            data = tmp;
            let id = rmp::decode::read_u16(&mut data).unwrap();
            packages.insert(id, name.to_owned());
        }

        // Read tags index
        let n_tags = rmp::decode::read_array_len(&mut data).unwrap();
        let mut results = 0;
        for _ in 0..n_tags {
            let (name, tmp) = rmp::decode::read_str_from_slice(data).unwrap();
            data = tmp;
            let found = name == exact;
            if found {
                body.push_str(&exact);
                results += 1;
            }
            let n_inst = rmp::decode::read_array_len(&mut data).unwrap();
            for _ in 0..n_inst {
                let id = rmp::decode::read_u16(&mut data).unwrap();
                if found {
                    body.push_str(" ");
                    body.push_str(&packages[&id]);
                }
            }
            if found {
                body.push_str("\n");
            }
        }

        body.push_str(&format!("\nQuery: {:?}\n", exact));
        body.push_str(&format!("Results: {}\n", results));
        body.push_str(&format!("Time: {:?}\n", start.elapsed()));

        (StatusCode::OK, body)
    }
}
