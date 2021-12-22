FROM ubuntu:focal

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update -yq && apt install -yq curl git
RUN (curl -sL https://golang.org/dl/go1.17.5.linux-amd64.tar.gz | tar xzC /usr/local)

COPY start /var/start
CMD ["/var/start"]
