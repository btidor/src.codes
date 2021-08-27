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
RUN DEBIAN_FRONTEND=noninteractive apt install -y -q \
    build-essential g++ libx11-dev libxkbfile-dev libsecret-1-dev \
    python-is-python3
