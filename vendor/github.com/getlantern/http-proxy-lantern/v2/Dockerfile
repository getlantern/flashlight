# This docker machine is able to compile http-proxy-lantern for Ubuntu Linux

FROM golang:1.18
# Note that we don't use alpine here because we need at least gcc.

RUN apt-get update && apt-get install -y build-essential pkg-config make libpcap-dev

ENV WORKDIR /src

# Expect the $WORKDIR volume to be mounted.
VOLUME [ "$WORKDIR" ]

WORKDIR $WORKDIR
