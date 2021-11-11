use fzf::Directory;
use fzf::Matcher;
use fzf::PathServer;
use fzf::Query;
use reqwest::Url;
use rouille::Response;
use std::collections::BinaryHeap;
use std::env;
use std::fs::File;
use std::io::Read;

const NUM_ITERATIONS: usize = 50;

fn main() {
    let args: Vec<String> = env::args().collect();
    if args.len() == 2 && args[1] == "--serve" {
        serve();
    } else if args.len() == 2 && args[1] == "--benchmark" {
        benchmark();
    } else {
        println!("usage: {} {{--serve | --benchmark}}", args[0]);
    }
}

fn serve() {
    let mut server = PathServer::new("dev".to_string(), 100);

    let mut file = File::open("paths.fzf").unwrap();
    let mut buf = Vec::new();
    file.read_to_end(&mut buf).unwrap();
    server.load("impish".to_string(), &buf);

    println!("Listening on 127.0.0.1:7070");
    rouille::start_server("127.0.0.1:7070", move |request| {
        let x = "https://127.0.0.1:7070".to_string() + &request.raw_url();
        let url = Url::parse(&x).unwrap();
        let (status, body) = server.handle(&url);

        Response::text(body).with_status_code(status.as_u16())
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
