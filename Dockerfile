# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang

# Build the flashlight command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go get -u gopkg.in/getlantern/flashlight.v2

# Run the flashlight command by default when the container starts.
ENTRYPOINT /go/bin/flashlight.v2 -role server -addr :62443

# Document that the service listens on port 62443.
EXPOSE 62443