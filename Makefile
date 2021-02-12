DISABLE_PORT_RANDOMIZATION ?=
GOBINDATA_BIN ?= $(shell which go-bindata)

SHELL := /bin/bash
SOURCES := $(shell find . -name '*[^_test].go')
BINARY_NAME ?= lantern

BUILD_RACE ?= '-race'
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
	if [[ ! -z "$$REPLICA" && ! -z "$$YINBI" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.EnableReplicaFeatures=true"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.EnableYinbiFeatures=true"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-replica.yaml.gz"; \
	elif [[ ! -z "$$REPLICA" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.EnableReplicaFeatures=true"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-replica.yaml.gz"; \
	elif [[ ! -z "$$YINBI" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.EnableYinbiFeatures=true"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-yinbi.yaml.gz"; \
	fi && \
	if [[ ! -z "$$NOREPLICA" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-no-replica.yaml.gz"; \
	fi && \
	BUILD_TAGS=$$(echo $$BUILD_TAGS | xargs) && echo "Build tags: $$BUILD_TAGS" && \
	EXTRA_LDFLAGS=$$(echo $$EXTRA_LDFLAGS | xargs) && echo "Extra ldflags: $$EXTRA_LDFLAGS"
endef

.PHONY: lantern beam update-icons vendor git-lfs

git-lfs:
	@if [ "$(shell which git-lfs)" = "" ]; then \
		echo "Missing Git LFS. See https://git-lfs.github.com" && exit 1; \
	fi

lantern: $(SOURCES)
	@$(call build-tags) && \
	BUILD_TAGS="$$BUILD_TAGS lantern" BINARY_NAME="lantern" EXTRA_LDFLAGS=$$EXTRA_LDFLAGS make app

beam: $(SOURCES)
	@$(call build-tags) && \
	BUILD_TAGS="$$BUILD_TAGS beam" BINARY_NAME="beam" EXTRA_LDFLAGS=$$EXTRA_LDFLAGS make app

windowscli: $(SOURCES)
	@$(call build-tags) && \
	BUILD_TAGS="$$BUILD_TAGS lantern" BINARY_NAME=$$BINARY_NAME-cli.exe EXTRA_LDFLAGS=$$EXTRA_LDFLAGS make windows

windowsgui: $(SOURCES)
	@$(call build-tags) && \
	BUILD_TAGS="$$BUILD_TAGS lantern" BINARY_NAME=$$BINARY_NAME-gui.exe EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -H=windowsgui" make windows

beam-windowscli: $(SOURCES)
	@$(call build-tags) && \
	BUILD_TAGS="$$BUILD_TAGS beam" BINARY_NAME="beam-cli.exe" EXTRA_LDFLAGS=$$EXTRA_LDFLAGS make windows

beam-windowsgui: $(SOURCES)
	@$(call build-tags) && \
	BUILD_TAGS="$$BUILD_TAGS beam" BINARY_NAME="beam-gui.exe" EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -H=windowsgui" make windows

windows:
	GOOS=windows GOARCH=386 BUILD_RACE='' CXX=i686-w64-mingw32-g++ CC=i686-w64-mingw32-gcc CGO_LDFLAGS="-static" EXTRA_LDFLAGS="$(LDFLAGS_NOSTRIP) $$EXTRA_LDFLAGS" make app

linux: $(SOURCES)
	@$(call build-tags) && \
	HEADLESS=true GOOS=linux GOARCH=amd64 BINARY_NAME=$$BINARY_NAME-linux BUILD_TAGS="$$BUILD_TAGS headless" make app

beam-linux: $(SOURCES)
	@$(call build-tags) && \
	HEADLESS=true GOOS=linux GOARCH=amd64 BINARY_NAME=beam-linux BUILD_TAGS="$$BUILD_TAGS headless" make app

app: | git-lfs
	GO111MODULE=on GOPRIVATE="github.com/getlantern" CGO_ENABLED=1 go build $(BUILD_RACE) -v -o $(BINARY_NAME) -tags="$$BUILD_TAGS" -ldflags="$$EXTRA_LDFLAGS -s " github.com/getlantern/flashlight/main;

# vendor installs vendored dependencies using go modules
vendor: | git-lfs
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

test-and-cover: $(SOURCES)
	@echo "mode: count" > profile.cov && \
	TP=$$(go list ./...) && \
	CP=$$(echo $$TP | tr ' ', ',') && \
	set -x && \
	for pkg in $$TP; do \
		GO111MODULE=on go test -race -v -tags="headless" -covermode=atomic -coverprofile=profile_tmp.cov -coverpkg "$$CP" $$pkg || exit 1; \
		tail -n +2 profile_tmp.cov >> profile.cov; \
	done

test: $(SOURCES)
	@TP=$$(go list ./... | grep -v /vendor/) && \
	for pkg in $$TP; do \
		GO111MODULE=on go test -failfast -race -v -tags="headless" $$pkg || exit 1; \
	done

clean:
	rm -f lantern
