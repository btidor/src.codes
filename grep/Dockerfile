FROM ubuntu:focal

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update -yq && apt install -yq curl git
RUN (curl -sL https://golang.org/dl/go1.17.6.linux-amd64.tar.gz | tar xzC /usr/local)

COPY grep/start /var/start
CMD ["/var/start"]
