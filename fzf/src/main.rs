use fzf::{Arena, Matcher, PathServer, Query};
use std::collections::BinaryHeap;
use std::env;
use std::fs::File;
use std::io::BufReader;
use std::str::FromStr;
use tiny_http::{Header, Response};
use url::Url;

const DISTRO: &str = "noble";
const MAX_RESULTS: usize = 100;
const NUM_ITERATIONS: usize = 50;

fn main() {
    let args: Vec<String> = env::args().collect();
    if args.len() == 1 {
        serve();
    } else if args.len() == 2 && args[1] == "--benchmark" {
        benchmark();
    } else {
        println!("usage: {} [--benchmark]", args[0]);
    }
}

fn serve() {
    let mut commit = env!("COMMIT").to_string();
    commit.truncate(8);

    let mut server = PathServer::new(commit, MAX_RESULTS);
    println!("Loading index from local cache");
    {
        let file = File::open("paths.fzf").unwrap();
        let mut buf = BufReader::new(file);
        server.load(DISTRO.to_string(), &mut buf);
    }

    let addr = "0.0.0.0:8080";
    println!("Starting server on {}", addr);
    let http = tiny_http::Server::http(addr).unwrap();

    loop {
        let request = match http.recv() {
            Ok(rq) => rq,
            Err(e) => {
                println!("error: {}", e);
                break;
            }
        };

        let url = Url::parse("https://127.0.0.1:9999")
            .unwrap()
            .join(request.url())
            .unwrap();
        let (status, body) = server.handle(&url);

        let mut response = Response::from_string(body).with_status_code(status);

        response.add_header(Header::from_str("Access-Control-Allow-Origin: *").unwrap());
        response.add_header(Header::from_str("Cache-Control: no-cache").unwrap());
        response
            .add_header(Header::from_str("Content-Security-Policy: default-src 'none';").unwrap());

        if let Err(e) = request.respond(response) {
            println!("error: {}", e)
        }
    }
}

fn benchmark() {
    // Load index
    let mut a = Arena::new();
    let directories;
    {
        let mut file = File::open("paths.fzf").unwrap();
        directories = a.load(&mut file).unwrap();
    }
    let query = Query::new("abseilabsl.c").unwrap();

    // Walk index
    for i in 0..NUM_ITERATIONS {
        println!("Iteration {}", i + 1);
        let mut h = BinaryHeap::new();
        for (_, directory) in directories.iter().enumerate() {
            Matcher::new(&query, 100, &a).walk(directory, "", &mut h, true);
        }
    }
}
