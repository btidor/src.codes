FROM golang:1.18 as grep
ARG COMMIT

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -v -o /go/build/ -ldflags "-X main.commit=${COMMIT}" ./...


FROM rust:1.66 as fzf
ARG COMMIT

WORKDIR /usr/src/myapp
COPY ./fzf/ .

RUN cargo build --release

FROM alpine:latest

WORKDIR /var/src.codes
COPY --from=grep /go/build/ .
COPY --from=fzf /usr/src/myapp/target/release/fzf .
COPY distributions.toml .
