FROM golang:1.22.6 AS builder

RUN apt-get update && apt-get install -y git git-lfs bsdmainutils

ARG TARGETOS
ARG TARGETARCH

ENV GOPRIVATE=github.com/getlantern/
RUN --mount=type=secret,id=github_token GH_TOKEN=$(cat /run/secrets/github_token) && git config --global url."https://${GH_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/"

WORKDIR /src

COPY . .

ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target="$GOCACHE" GOOS=$TARGETOS GOARCH=$TARGETARCH go mod tidy
RUN --mount=type=cache,target="$GOCACHE" GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /tester ./tester/...

FROM debian:bookworm-slim

COPY --from=builder /tester /tester

ENV TRACE=true
ENTRYPOINT ["/tester"]
