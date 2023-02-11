SOURCES := $(shell find . -type f -name "*.go" | grep -v /vendor)

DEPS := go.mod go.sum

makesessions: $(SOURCES) $(DEPS)
	GO111MODULE=on GOOS=linux GOARCH=amd64 go build github.com/getlantern/tlsresumption/cmd/makesessions && upx makesessions