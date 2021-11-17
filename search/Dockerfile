FROM livegrep/base

ENV DEBIAN_FRONTEND=noninteractive

# Install system tools
RUN apt update -yq && apt install -yq ubuntu-minimal ca-certificates

# Install Caddy
COPY caddy.gpg /etc/apt/trusted.gpg.d/
COPY caddy.list /etc/apt/sources.list.d/

RUN apt update -yq && apt install -yq caddy

# Run livegrep
COPY launch-search /usr/local/bin/
COPY Caddyfile /var/local

CMD ["/usr/local/bin/launch-search"]
