FROM ubuntu:focal

ENV DEBIAN_FRONTEND=noninteractive

# Install system tools.
RUN apt update -yq && apt install -yq ca-certificates curl git iproute2 iputils-ping

# Install Bazel. This is a separate step because we need ca-certificates
# before `apt update` will work with the Bazel repositories.
COPY bazel.gpg /etc/apt/trusted.gpg.d/
COPY bazel.list /etc/apt/sources.list.d/

RUN apt update -yq && apt install -yq bazel bazel-4.0.0 && \
    git clone https://github.com/livegrep/livegrep /var/livegrep && \
    (cd /var/livegrep && bazel build //...)

CMD ["/bin/bash"]
