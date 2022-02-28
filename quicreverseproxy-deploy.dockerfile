FROM alpine:latest
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

COPY . /app
WORKDIR /app
ENTRYPOINT go run cmd/quicreverseproxy/main.go -port=8080
