# This docker machine is able to compile http-proxy-lantern for Ubuntu Linux

FROM ubuntu:20.04

# Avoids the build hanging on anything that might expect user interaction.
ARG DEBIAN_FRONTEND=noninteractive

# Requisites for building Go.
RUN apt-get update && apt-get install -y git tar gzip curl hostname

# Compilers and tools for CGO.
RUN apt-get install -y build-essential pkg-config make libpcap-dev

# Getting Go.
ENV GOROOT /usr/local/go
ENV GOPATH /

ENV PATH $PATH:$GOROOT/bin

ARG go_version
ENV GO_VERSION $go_version
ENV GO_PACKAGE_URL https://storage.googleapis.com/golang/$GO_VERSION.linux-amd64.tar.gz
RUN curl -sSL $GO_PACKAGE_URL | tar -xvzf - -C /usr/local

ENV WORKDIR /src

# Expect the $WORKDIR volume to be mounted.
VOLUME [ "$WORKDIR" ]

WORKDIR $WORKDIR
