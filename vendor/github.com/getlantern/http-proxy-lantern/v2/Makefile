SHELL := /bin/bash
.DEFAULT_GOAL := build
GIT_REVISION := $(shell git rev-parse --short HEAD)
CHANGE_BIN   := $(shell which git-chglog)

# Binaries compiled for the host OS will be output to BUILD_DIR.
# Binaries compiles for distribution will be output to DIST_DIR.
BUILD_DIR   := bin
DIST_DIR    := dist-bin

SRCS := $(shell find . -name "*.go" -not -path "*_test.go" -not -path "./vendor/*") go.mod go.sum

GO_VERSION := 1.17.7

DOCKER_IMAGE_TAG := http-proxy-builder
DOCKER_VOLS = "-v $$PWD/../../..:/src"

get-command = $(shell which="$$(which $(1) 2> /dev/null)" && if [[ ! -z "$$which" ]]; then printf %q "$$which"; fi)

DOCKER    := $(call get-command,docker)
GO        := $(call get-command,go)

# We can only build natively on Linux. This is because we cross-compile for Linux and some
# dependencies rely on C libraries like libpcap-dev.
BUILD_WITH_DOCKER = false

# Controls whether logs from Redis are included in test output.
REDIS_LOGS ?= false

ifeq ($(OS),Windows)
	BUILD_WITH_DOCKER = true
else ifeq ($(shell uname -s),Darwin)
	BUILD_WITH_DOCKER = true
endif

BUILD_TYPE = stable
ifeq ($(BUILD_CANARY),true)
	BUILD_TYPE = canary
endif

.PHONY: build dist distnochange dist-on-linux dist-on-docker clean test system-checks

# This tags the current version and creates a CHANGELOG for the current directory.
define tag-changelog
	echo "Tagging..." && \
	git tag -a "$$VERSION" -f --annotate -m"Tagged $$VERSION" && \
	git push --tags -f && \
	$(CHANGE_BIN) --output CHANGELOG.md && \
	git add CHANGELOG.md && \
	git commit -m "Updated changelog for $$VERSION" && \
	git push origin HEAD
endef

guard-%:
	 @ if [ -z '${${*}}' ]; then echo 'Environment variable $* not set' && exit 1; fi

require-version: guard-VERSION
	@if ! [[ "$$VERSION" =~ v[0-9]+[.][0-9]+[.][0-9]+ ]]; then \
		echo "VERSION must be a semantic version like 'v1.2.10'"; \
		exit 1; \
	fi

require-change:
	@ if [ "$(CHANGE_BIN)" = "" ]; then \
		echo 'Missing "git-chglog" command. See https://github.com/git-chglog/git-chglog'; exit 1; \
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

dist-on-linux: $(DIST_DIR)
	GOOS=linux GOARCH=amd64 GO111MODULE=on GOPRIVATE="github.com/getlantern" \
	go build -o $(DIST_DIR)/http-proxy \
	-ldflags="-X main.revision=$(GIT_REVISION) -X main.build_type=$(BUILD_TYPE)" \
	./http-proxy

dist-on-docker: $(DIST_DIR) docker-builder
	GO111MODULE=on go mod vendor && \
	docker run -e GIT_REVISION='$(GIT_REVISION)' -e BUILD_TYPE='$(BUILD_TYPE)' \
	-v $$PWD:/src -t $(DOCKER_IMAGE_TAG) /bin/bash -c \
	'cd /src && go build -o $(DIST_DIR)/http-proxy -ldflags="-X main.revision=$$GIT_REVISION -X main.build_type=$$BUILD_TYPE" -mod=vendor ./http-proxy'

$(DIST_DIR)/http-proxy: $(SRCS)
	@if [ "$(BUILD_WITH_DOCKER)" = "true" ]; then \
		$(MAKE) dist-on-docker; \
	else \
		$(MAKE) dist-on-linux; \
	fi

distnochange: $(DIST_DIR)/http-proxy

dist: require-version require-change $(DIST_DIR)/http-proxy
	$(call tag-changelog)

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
	@if [[ -z "$(DOCKER)" ]]; then echo 'Missing "docker" command.'; exit 1; fi && \
	if [[ -z "$(GO)" ]]; then echo 'Missing "go" command.'; exit 1; fi

docker-builder: system-checks
	DOCKER_CONTEXT=.$(DOCKER_IMAGE_TAG)-context && \
	mkdir -p $$DOCKER_CONTEXT && \
	cp Dockerfile $$DOCKER_CONTEXT && \
	docker build -t $(DOCKER_IMAGE_TAG) --build-arg go_version=go$(GO_VERSION) $$DOCKER_CONTEXT

test:
	./test.bash $(REDIS_LOGS)
