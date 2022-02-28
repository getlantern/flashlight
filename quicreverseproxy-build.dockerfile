# XXX <15-11-21, soltzen> Alpine linux is very light and uses musl by
# default, which is very important so as not to get glibc
# incompatibility issues when running the binary
FROM alpine:3.14
MAINTAINER "The Lantern Team" <team@getlantern.org>

RUN apk update && apk add --no-cache \
  make \
  libpcap-dev \
  bash \
  musl-dev \
  g++ \
  git \
  tar \
  gzip \
  curl \
  go \
  openssh

ENV GOROOT /usr/lib/go
ENV GOPATH /go
ENV PATH /go/bin:$PATH
ENV GO_VERSION $go_version

ENV WORKDIR /src

VOLUME [ "$WORKDIR" ]

WORKDIR $WORKDIR
