FROM ubuntu:focal

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update -yq && apt install -yq curl
RUN (curl -sL https://golang.org/dl/go1.17.5.linux-amd64.tar.gz | tar xzC /usr/local)

WORKDIR /var/src.codes
CMD ["/usr/local/go/bin/go", "run", "./grep", "serve", \
    "-cert", "/data/tls/certificate.pem", "-key", "/data/tls/key.pem"]
