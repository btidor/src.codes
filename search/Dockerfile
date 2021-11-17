FROM livegrep/base

ENV DEBIAN_FRONTEND=noninteractive

# Install system tools
RUN apt update -yq && apt install -yq ubuntu-minimal ca-certificates

# Install Caddy
COPY caddy.gpg /etc/apt/trusted.gpg.d/
COPY caddy.list /etc/apt/sources.list.d/

RUN apt update -yq && apt install -yq caddy

CMD ["/bin/bash"]
