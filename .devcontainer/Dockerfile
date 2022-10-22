FROM mcr.microsoft.com/vscode/devcontainers/base:0-jammy

ENV DEBIAN_FRONTEND=noninteractive

# Pre-install VS Code build dependencies (used by vscode/build) so they're
# cached in a layer.
RUN apt update -q && \
    apt install -y -q build-essential g++ libx11-dev libxkbfile-dev \
        libsecret-1-dev python-is-python3

# Set up third-party APT repositories
COPY *.gpg /usr/share/keyrings/
COPY *.list /etc/apt/sources.list.d/

# Install Node and npm packages for extension development. VS Code uses Node
# 16.x: https://github.com/microsoft/vscode/wiki/How-to-Contribute
RUN apt update -q && \
    apt install -y -q nodejs && \
    npm install -g npm yo generator-code

# Install development tools
RUN apt update -q && \
    apt install -y -q silversearcher-ag mysql-client graphviz && \
    wget https://github.com/planetscale/cli/releases/download/v0.122.0/pscale_0.122.0_linux_amd64.deb && \
    dpkg --install pscale_0.122.0_linux_amd64.deb && \
    apt install -y -q linux-tools-generic google-perftools \
        libgoogle-perftools-dev llvm

# Install Go
RUN (curl -sL https://go.dev/dl/go1.19.2.linux-amd64.tar.gz | tar xzC /usr/local) && \
    (echo 'PATH=$PATH:/usr/local/go/bin' >> /home/vscode/.profile)

# Install Rust
RUN su vscode -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y" && \
    apt update -q && \
    apt install -y -q libssl-dev gnuplot-nox

# Install ctags
RUN apt update -q && apt install -y -q universal-ctags
