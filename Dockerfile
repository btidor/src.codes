### Build grep
FROM golang:1.22 as grep
ARG COMMIT

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -v -o /go/build/ -ldflags "-X main.commit=${COMMIT}" ./...

### Build fzf
FROM rust:1.79 as fzf
ARG COMMIT

WORKDIR /usr/src/myapp
COPY ./fzf/ .

RUN apt update -q && apt install -y -q musl-tools
RUN rustup target add x86_64-unknown-linux-musl

RUN RUSTFLAGS="-C target-cpu=znver2" cargo build --release --target=x86_64-unknown-linux-musl

### Assemble production image
FROM alpine:latest

WORKDIR /var/src.codes

COPY --from=grep /go/build/grep .
COPY --from=fzf /usr/src/myapp/target/x86_64-unknown-linux-musl/release/fzf .
COPY distributions.toml .
