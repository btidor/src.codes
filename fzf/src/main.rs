use fzf::Directory;
use fzf::Matcher;
use fzf::PathServer;
use fzf::Query;
use reqwest::Url;
use std::collections::BinaryHeap;
use std::env;
use std::fs;
use std::fs::File;
use std::io::Read;
use std::path::Path;
use std::str::FromStr;
use tiny_http::Header;
use tiny_http::Response;

const DISTRO: &str = "kinetic";
const MAX_RESULTS: usize = 100;
const META_BASE: &str = "https://meta.src.codes/";
const NUM_ITERATIONS: usize = 50;

fn main() {
    let args: Vec<String> = env::args().collect();
    if args.len() == 3 && args[1] == "--serve" {
        serve(args[2].to_string(), false);
    } else if args.len() == 2 && args[1] == "--dev" {
        serve("localhost:7070".to_string(), true);
    } else if args.len() == 2 && args[1] == "--benchmark" {
        benchmark();
    } else {
        println!(
            "usage: {} {{--serve SOCKET | --dev | --benchmark}}",
            args[0]
        );
    }
}

fn serve(addr: String, local: bool) {
    let mut commit = env!("COMMIT").to_string();
    commit.truncate(8);

    let mut server = PathServer::new(commit, MAX_RESULTS);
    let mut buf = Vec::new();
    let resp;

    if local {
        println!("Loading index from local cache");
        let mut file = File::open("paths.fzf").unwrap();
        file.read_to_end(&mut buf).unwrap();
        server.load(DISTRO.to_string(), &buf);
    } else {
        println!("Downloading index");
        let url = META_BASE.to_string() + DISTRO + "/paths.fzf";
        resp = reqwest::blocking::get(url).unwrap().bytes().unwrap();
        server.load(DISTRO.to_string(), &resp);
    }

    println!("Starting server on {}", addr);
    let http = if local {
        tiny_http::Server::http(addr).unwrap()
    } else {
        let path = Path::new(&addr);
        if path.exists() {
            fs::remove_file(path).unwrap();
        }
        tiny_http::Server::http_unix(path).unwrap()
    };

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

        let mut response = Response::from_string(body).with_status_code(status.as_u16());

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
    let mut file = File::open("paths.fzf").unwrap();
    let mut buf = Vec::new();
    file.read_to_end(&mut buf).unwrap();

    let directories = Directory::load(&buf[..]).unwrap();
    let query = Query::new("abseilabsl.c").unwrap();

    // Walk index
    for i in 0..NUM_ITERATIONS {
        println!("Iteration {}", i + 1);
        let mut h = BinaryHeap::new();
        for (_, directory) in directories.iter().enumerate() {
            Matcher::new(&query, 100).walk(directory, "", &mut h);
        }
    }
}
