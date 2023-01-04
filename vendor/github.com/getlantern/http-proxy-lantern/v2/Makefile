SHELL := bash
.DEFAULT_GOAL := build
GIT_REVISION := $(shell git rev-parse --short HEAD)

# Binaries compiled for the host OS will be output to BUILD_DIR.
# Binaries compiles for distribution will be output to DIST_DIR.
BUILD_DIR   := bin
DIST_DIR    := dist-bin
BINARY      := $(DIST_DIR)/http-proxy

SRCS := $(shell find . -name "*.go" -not -path "*_test.go" -not -path "./vendor/*") go.mod go.sum

get-command = $(shell which="$$(which $(1) 2> /dev/null)" && if [[ ! -z "$$which" ]]; then printf %q "$$which"; fi)

GO        := $(call get-command,go)

# Controls whether logs from Redis are included in test output.
REDIS_LOGS ?= false

BUILD_TYPE = stable
ifeq ($(BUILD_CANARY),true)
	BUILD_TYPE = canary
endif

.PHONY: build dist distnochange clean test system-checks

guard-%:
	 @ if [ -z '${${*}}' ]; then echo 'Environment variable $* not set' && exit 1; fi

require-version: guard-VERSION
	@if ! [[ "$$VERSION" =~ v[0-9]+[.][0-9]+[.][0-9]+ ]]; then \
		echo "VERSION must be a semantic version like 'v1.2.10'"; \
		exit 1; \
	fi

# n.b. The http-proxy-custom prefix is to facilitate searching for custom binaries in the S3 UI.
require-binary-name: guard-BINARY_NAME
	@if ! [[ "$$BINARY_NAME" =~ http-proxy-custom-.+-.+ ]]; then \
		echo "BINARY_NAME must be a name of the form 'http-proxy-custom-<your name or alias>-<issue number>'"; \
		echo "For example, 'http-proxy-custom-hwh33-tlsmasq999'"; \
		exit 1; \
	fi

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(DIST_DIR):
	mkdir -p $(DIST_DIR)

$(BUILD_DIR)/http-proxy: $(SRCS) | $(BUILD_DIR)
	GOPRIVATE="github.com/getlantern" go build -o $(BUILD_DIR) ./http-proxy

build: $(BUILD_DIR)/http-proxy

local-rts: build
	./bin/http-proxy -config ./rts/rts.ini

local-proxy: local-rts

$(DIST_DIR)/http-proxy: $(SRCS)
	GOOS=linux GOARCH=amd64 GO111MODULE=on GOPRIVATE="github.com/getlantern" \
	go build -o $(DIST_DIR)/http-proxy \
	-ldflags="-X main.revision=$(GIT_REVISION) -X main.build_type=$(BUILD_TYPE)" \
	./http-proxy

distnochange: $(DIST_DIR)/http-proxy

dist: require-version $(DIST_DIR)/http-proxy
	echo "Tagging..." && \
	git tag -a "$$VERSION" -f --annotate -m"Tagged $$VERSION" && \
	git push --tags

deploy: $(DIST_DIR)/http-proxy
	s3cmd put $(DIST_DIR)/http-proxy s3://http-proxy/http-proxy && \
	s3cmd setacl --acl-grant read:f87080f71ec0be3b9a933cbb244a6c24d4aca584ac32b3220f56d59071043747 s3://http-proxy/http-proxy

deploy-staging: $(DIST_DIR)/http-proxy
	s3cmd put $(DIST_DIR)/http-proxy s3://http-proxy/http-proxy-staging && \
	s3cmd setacl --acl-grant read:f87080f71ec0be3b9a933cbb244a6c24d4aca584ac32b3220f56d59071043747 s3://http-proxy/http-proxy-staging

deploy-canary: $(DIST_DIR)/http-proxy
	s3cmd put $(DIST_DIR)/http-proxy s3://http-proxy/http-proxy-canary && \
	s3cmd setacl --acl-grant read:f87080f71ec0be3b9a933cbb244a6c24d4aca584ac32b3220f56d59071043747 s3://http-proxy/http-proxy-canary

# See 'Deploying a Custom Binary' in the README.
deploy-custom: $(DIST_DIR)/http-proxy require-binary-name
	s3cmd put $(DIST_DIR)/http-proxy s3://http-proxy/$(BINARY_NAME) && \
	s3cmd setacl --acl-grant read:f87080f71ec0be3b9a933cbb244a6c24d4aca584ac32b3220f56d59071043747 s3://http-proxy/$(BINARY_NAME)

clean:
	rm -rf $(BUILD_DIR) $(DIST_DIR)

system-checks:
	@if [[ -z "$(GO)" ]]; then echo 'Missing "go" command.'; exit 1; fi

test:
	./test.bash $(REDIS_LOGS)
