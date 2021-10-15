use fzf::Server;
use reqwest::Url;
use std::env;
use std::error::Error;
use vercel_lambda::{error::VercelError, lambda, Request, Response};

// TODO: can only support one distro under startup timeout
const DISTROS: &[&str] = &["impish"];
const MAX_RESULTS: usize = 100;
const META_BASE: &str = "https://meta.src.codes/";

// Start the runtime with the handler
fn main() -> Result<(), Box<dyn Error>> {
	let commit = env::var("VERCEL_GIT_COMMIT_SHA").unwrap()[..8].to_string();
	let mut server = Server::new(commit, MAX_RESULTS);

	for distro in DISTROS {
		let url = META_BASE.to_string() + distro + "/paths.fzf";
		let resp = reqwest::blocking::get(url).unwrap().bytes().unwrap();
		server.load(distro.to_string(), &resp);
	}

	// TODO: invalidate cache

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
