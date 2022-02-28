binary_name := "replica-p2p-quic-reverse-proxy"
id_u := `id -u`
id_g := `id -g`

deploy-cmd-quicreverseproxy-to-flyio: clean check-env
    go mod vendor
    flyctl deploy --config quicreverseproxy-fly.toml

build-prod-with-docker: clean check-env
    @# Vendor so as not to worry about ssh agent forwarding
    go mod vendor
    docker build -t {{ binary_name }} -f quicreverseproxy-build.dockerfile .
    docker run \
    -v $PWD:/src \
    -it {{ binary_name }} /bin/bash -c \
    'cd /src && \
    go env -w "GOPRIVATE=github.com/getlantern/*" && \
    go build -o {{ binary_name }} -ldflags="-extldflags -static -s -w" cmd/quicreverseproxy/main.go && \
    chown -R {{ id_u }}:{{ id_g }} {{ binary_name }}'

# Cleans build artifacts
clean:
    rm -rf vendor

check-env:
    @command -v docker >/dev/null 2>&1 || { echo "Docker is not installed"; exit 1; }
    @command -v flyctl >/dev/null 2>&1 || { echo "flyctl is not installed"; exit 1; }
    @command -v go >/dev/null 2>&1 || { echo "go is not installed"; exit 1; }
