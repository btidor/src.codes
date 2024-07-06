# syntax = btidor/syntax

FROM mcr.microsoft.com/devcontainers/base:noble

COPY *.sources /etc/apt/sources.list.d/

# Pre-install VS Code build dependencies (used by vscode/build) so they're
# cached in a layer.
ADD --apt build-essential g++ libx11-dev libxkbfile-dev libsecret-1-dev \
    python-is-python3

# Install Node and npm packages for extension development. VS Code uses Node
# 18.x: https://github.com/microsoft/vscode/wiki/How-to-Contribute
ADD --apt nodejs
RUN npm install -g npm yo generator-code

# Install development packages
ADD --apt graphviz linux-tools-generic google-perftools libgoogle-perftools-dev \
    llvm gnuplot-nox musl-tools universal-ctags postgresql-client lftp

RUN curl -L https://fly.io/install.sh | FLYCTL_INSTALL=/usr/local sh

# Install Go
RUN (curl -sL https://go.dev/dl/go1.22.5.linux-amd64.tar.gz | tar xzC /usr/local) && \
    (echo 'PATH=$PATH:/usr/local/go/bin' >> /home/vscode/.profile)

# Install Rust
RUN su vscode -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y"
