DISABLE_PORT_RANDOMIZATION ?=
GOBINDATA_BIN ?= $(shell which go-bindata)

SHELL := /bin/bash
SOURCES := $(shell find . -name '*[^_test].go')
BINARY_NAME := lantern

.PHONY: lantern update-icons

BUILD_RACE := '-race'
REVISION_DATE := $(shell git log -1 --pretty=format:%ad --date=format:%Y%m%d.%H%M%S)
BUILD_DATE := $(shell date -u +%Y%m%d.%H%M%S)

ifeq ($(OS),Windows_NT)
	  # Race detection is not supported by Go Windows 386, so disable it. The -x
		# is just a hack to allow us to pass something in place of -race below.
		BUILD_RACE = '-x'
endif

define build-tags
	BINARY_NAME="$(BINARY_NAME)" && \
	BUILD_TAGS="$(BUILD_TAGS)" && \
	EXTRA_LDFLAGS="-X github.com/getlantern/flashlight/common.RevisionDate=$(REVISION_DATE) -X github.com/getlantern/flashlight/common.BuildDate=$(BUILD_DATE) " && \
	if [[ ! -z "$$VERSION" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.CompileTimePackageVersion=$$VERSION"; \
	else \
		echo "** VERSION was not set, using default version. This is OK while in development."; \
	fi && \
	if [[ ! -z "$$HEADLESS" ]]; then \
		BINARY_NAME="$$BINARY_NAME-headless"; \
		BUILD_TAGS="$$BUILD_TAGS headless"; \
	fi && \
	if [[ ! -z "$$DISABLE_PORT_RANDOMIZATION" ]]; then \
		BUILD_TAGS="$$BUILD_TAGS disableresourcerandomization"; \
	fi && \
	if [[ ! -z "$$STAGING" ]]; then \
		BINARY_NAME="$$BINARY_NAME-staging"; \
		BUILD_TAGS="$$BUILD_TAGS staging"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.StagingMode=$$STAGING"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/lantern.StagingMode=$$STAGING"; \
	fi && \
	BUILD_TAGS=$$(echo $$BUILD_TAGS | xargs) && echo "Build tags: $$BUILD_TAGS" && \
	EXTRA_LDFLAGS=$$(echo $$EXTRA_LDFLAGS | xargs) && echo "Extra ldflags: $$EXTRA_LDFLAGS"
endef

.PHONY: vendor novendor

lantern: $(SOURCES)
	@$(call build-tags) && \
	CGO_ENABLED=1 go build $(BUILD_RACE) -o $$BINARY_NAME -tags="$$BUILD_TAGS" -ldflags="$$EXTRA_LDFLAGS -s" github.com/getlantern/flashlight/main;

windowscli: $(SOURCES)
	@$(call build-tags) && \
	GOOS=windows GOARCH=386 CGO_ENABLED=1 go build -o $$BINARY_NAME-cli.exe -tags="$$BUILD_TAGS" -ldflags="$(LDFLAGS_NOSTRIP) $$EXTRA_LDFLAGS" github.com/getlantern/flashlight/main;

windowsgui: $(SOURCES)
	@$(call build-tags) && \
	GOOS=windows GOARCH=386 CGO_ENABLED=1 go build -a -o $$BINARY_NAME-gui.exe -tags="$$BUILD_TAGS" -ldflags="$(LDFLAGS_NOSTRIP) $$EXTRA_LDFLAGS -H=windowsgui" github.com/getlantern/flashlight/main;

linux: $(SOURCES)
	@$(call build-tags) && \
	HEADLESS=true GOOS=linux GOARCH=amd64 go build -o $$BINARY_NAME-linux -tags="$$BUILD_TAGS headless" -ldflags="$$EXTRA_LDFLAGS" github.com/getlantern/flashlight/main;

# vendor installs vendored dependencies using Dep
vendor:
	GO111MODULE=on go mod vendor

# test go-bindata dependency and update icons if up-to-date
update-icons:
	@if [[ -z "$(GOBINDATA_BIN)" ]]; then \
		echo 'Missing dependency go-bindata, get with: go get -u github.com/kevinburke/go-bindata/...' && exit 1; \
	fi; \
	GOBINDATA_VERSION=`grep jteeuwen $(GOBINDATA_BIN)`; \
	if [[ ! -z "$$GOBINDATA_VERSION" ]]; then \
		echo 'Update dependency go-bindata with: go get -u github.com/kevinburke/go-bindata/...' && exit 1; \
	fi; \
	$(GOBINDATA_BIN) -nomemcopy -nocompress -pkg icons -o icons/icons.go -prefix icons -ignore icons.go icons

# novendor removes the vendor folder to allow building with whatever is on your
# GOPATH
novendor:
	@rm -Rf vendor

test-and-cover: $(SOURCES)
	@echo "mode: count" > profile.cov && \
	TP=$$(find . -name "*_test.go" -printf '%h\n' | grep  -v vendor | sort -u) && \
	CP=$$(echo $$TP | tr ' ', ',') && \
	set -x && \
	for pkg in $$TP; do \
		go test -race -v -tags="headless" -covermode=atomic -coverprofile=profile_tmp.cov -coverpkg "$$CP" $$pkg || exit 1; \
		tail -n +2 profile_tmp.cov >> profile.cov; \
	done

test: $(SOURCES)
	@TP=$$(go list ./... | grep -v /vendor/) && \
	go test -race -v -tags="headless" $$TP || exit 1; \

clean:
	rm -f lantern
