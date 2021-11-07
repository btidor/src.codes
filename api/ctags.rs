use fzf::TagServer;
use reqwest::Url;
use std::env;
use std::error::Error;
use vercel_lambda::{error::VercelError, lambda, Request, Response};

const META_BASE: &str = "https://meta.src.codes/";

// Start the runtime with the handler
fn main() -> Result<(), Box<dyn Error>> {
    let commit = env::var("VERCEL_GIT_COMMIT_SHA").unwrap()[..8].to_string();

    let distro = "impish".to_string(); // TODO: make dynamic

    let url = META_BASE.to_string() + &distro + "/tags.fzf";
    let resp = reqwest::blocking::get(url).unwrap().bytes().unwrap();
    let server = TagServer::new(commit, distro.to_string(), resp.to_vec());

    Ok(lambda!(
        move |req: Request| -> Result<Response<String>, VercelError> {
            let url = Url::parse(&req.uri().to_string()).unwrap();
            let (status, body) = server.handle(&url);

            Ok(Response::builder()
                .status(status)
                .header("Content-Type", "text/plain")
                .body(body)
                .unwrap())
        }
    ))
}