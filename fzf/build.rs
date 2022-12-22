fn main() {
    println!("cargo:rerun-if-env-changed=COMMIT");
    let commit = match std::env::var("COMMIT") {
        Ok(v) => v,
        Err(_) => String::from("dev"),
    };
    println!("cargo:rustc-env=COMMIT={}", commit);
}
