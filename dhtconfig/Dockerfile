# syntax=docker/dockerfile:1.3

FROM alpine:latest

# alpine doesn't have go 1.18 yet

ARG GO_VERSION=1.18
ENV GO_PACKAGE_FILE=go$GO_VERSION.linux-amd64.tar.gz
ARG GO_PACKAGE_URL=https://go.dev/dl/$GO_PACKAGE_FILE
ADD $GO_PACKAGE_URL ./
RUN tar -xzf $GO_PACKAGE_FILE -C /
ENV PATH="/go/bin:$PATH"
ENV GOROOT=/go
RUN apk add gcompat

WORKDIR /app
RUN apk add make gcc
RUN apk add musl-dev
RUN apk add g++
RUN apk add curl
COPY Makefile .
ENV GOWORK=off
ENV GOMODCACHE=/gomodcache
ENV GOCACHE=/gocache
RUN \
	--mount=type=cache,target=$GOMODCACHE \
	--mount=type=cache,target=$GOCACHE \
	make deps
# TODO take from secrets/env
COPY dht-private-key .
ENTRYPOINT ["make"]
CMD ["publish", "seed"]
