FROM golang:1.24.2 AS builder

RUN apt-get update && apt-get install -y git git-lfs bsdmainutils

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY . .

ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target="$GOCACHE" GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /tester ./tester/...

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates curl && rm -rf /var/lib/apt/lists/*

COPY --from=builder /tester /tester

ENV TRACE=true
ENTRYPOINT ["/tester"]
