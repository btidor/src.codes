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
use std::sync::Arc;

const DISTROS: &[&str] = &["kinetic"];
const MAX_RESULTS: usize = 100;
const META_BASE: &str = "https://meta.src.codes/";
const NUM_ITERATIONS: usize = 50;

fn main() {
    let args: Vec<String> = env::args().collect();
    if args.len() == 3 && args[1] == "--serve" {
        serve_prod(args[2].to_string());
    } else if args.len() == 2 && args[1] == "--serve-dev" {
        serve_dev();
    } else if args.len() == 2 && args[1] == "--benchmark" {
        benchmark();
    } else {
        println!(
            "usage: {} {{--serve SOCKET | --serve-dev | --benchmark}}",
            args[0]
        );
    }
}

fn serve_prod(socket: String) {
    let mut commit = env!("COMMIT").to_string();
    commit.truncate(8);
    let mut server = PathServer::new(commit, MAX_RESULTS);

    for distro in DISTROS {
        println!("Downloading {} index", distro);
        let url = META_BASE.to_string() + distro + "/paths.fzf";
        let resp = reqwest::blocking::get(url).unwrap().bytes().unwrap();
        server.load(distro.to_string(), &resp);
    }

    println!("Starting server on {}", socket);
    let path = Path::new(&socket);
    if path.exists() {
        fs::remove_file(path).unwrap();
    }
    let http = Arc::new(tiny_http::Server::http_unix(path).unwrap());
    let server = Arc::new(server);

    for _ in 0..4 {
        let http = http.clone();
        let server = server.clone();
        std::thread::spawn(move || loop {
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

            let response = tiny_http::Response::from_string(body).with_status_code(status.as_u16());
            if let Err(e) = request.respond(response) {
                println!("error: {}", e)
            }
        });
    }
    loop {}
}

fn serve_dev() {
    let mut commit = env!("COMMIT").to_string();
    commit.truncate(8);
    let mut server = PathServer::new(commit, MAX_RESULTS);

    let mut file = File::open("paths.fzf").unwrap();
    let mut buf = Vec::new();
    file.read_to_end(&mut buf).unwrap();
    server.load("kinetic".to_string(), &buf);

    println!("Listening on 127.0.0.1:7070");
    rouille::start_server("127.0.0.1:7070", move |request| {
        let url = Url::parse("https://127.0.0.1:7070")
            .unwrap()
            .join(request.raw_url())
            .unwrap();
        let (status, body) = server.handle(&url);

        rouille::Response::text(body).with_status_code(status.as_u16())
    });
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
