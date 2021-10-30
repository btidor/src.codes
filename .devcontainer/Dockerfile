FROM mcr.microsoft.com/vscode/devcontainers/base:0-focal

ENV DEBIAN_FRONTEND=noninteractive

# Install Node and npm packages for extension development. VS Code uses Node
# 14.x: https://github.com/microsoft/vscode/wiki/How-to-Contribute
RUN (curl -sL https://deb.nodesource.com/setup_14.x | bash) && \
    apt update -q && \
    apt install -y -q nodejs && \
    npm install -g yo generator-code

# Pre-install VS Code build dependencies (used by vscode/build) so they're
# cached in a layer.
RUN apt install -y -q build-essential g++ libx11-dev libxkbfile-dev \
        libsecret-1-dev python-is-python3

# Install development tools
RUN apt install -y -q silversearcher-ag mysql-client graphviz && \
    wget https://github.com/planetscale/cli/releases/download/v0.72.0/pscale_0.72.0_linux_amd64.deb && \
    dpkg --install pscale_0.72.0_linux_amd64.deb && \
    apt install -y -q linux-tools-generic google-perftools \
        libgoogle-perftools-dev llvm

# Install Go
RUN (curl -sL https://golang.org/dl/go1.17.2.linux-amd64.tar.gz | tar xzC /usr/local) && \
    (echo 'PATH=$PATH:/usr/local/go/bin' >> /home/vscode/.profile)

# Install Rust
RUN su vscode -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y" && \
    apt install -y -q libssl-dev gnuplot

# Install ctags
RUN apt install -y -q universal-ctags
