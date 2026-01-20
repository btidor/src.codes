### Build fzf
FROM rust:1.92 AS build
ARG COMMIT

WORKDIR /usr/src/myapp
COPY . .

RUN apt update -q && apt install -y -q musl-tools
RUN rustup target add x86_64-unknown-linux-musl

RUN RUSTFLAGS="-C target-cpu=znver2" cargo build --release --target=x86_64-unknown-linux-musl

### Assemble production image
FROM alpine:latest

WORKDIR /var/src.codes
COPY --from=build /usr/src/myapp/target/x86_64-unknown-linux-musl/release/fzf .

ADD https://meta.src.codes/noble/paths.fzf .

ENTRYPOINT ["/var/src.codes/fzf"]
