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
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-replica.yaml.gz"; \
	elif [[ ! -z "$$REPLICA" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-replica.yaml.gz"; \
	elif [[ ! -z "$$YINBI" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-yinbi.yaml.gz"; \
	elif [[ ! -z "$$TRAFFICLOG" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/config.EnableTrafficlogFeatures=true"; \
	fi && \
	if [[ ! -z "$$NOREPLICA" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.GlobalURL=https://globalconfig.flashlightproxy.com/global-no-replica.yaml.gz"; \
	fi && \
	BUILD_TAGS=$$(echo $$BUILD_TAGS | xargs) && echo "Build tags: $$BUILD_TAGS" && \
	EXTRA_LDFLAGS=$$(echo $$EXTRA_LDFLAGS | xargs) && echo "Extra ldflags: $$EXTRA_LDFLAGS"
endef

.PHONY: vendor git-lfs

git-lfs:
	@if [ "$(shell which git-lfs)" = "" ]; then \
		echo "Missing Git LFS. See https://git-lfs.github.com" && exit 1; \
	fi
	@if ! git config -l | grep -q "filter.lfs"; then \
		echo "git-lfs installation is incomplete. Run 'git lfs install'." && exit 1; \
	fi

# vendor installs vendored dependencies using go modules
vendor: | git-lfs
	GO111MODULE=on go mod vendor

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

define prep-for-mobile
	go mod vendor && \
	GO111MODULE=off go get -u golang.org/x/mobile/cmd/gomobile && \
	GO111MODULE=off go get -u golang.org/x/mobile/cmd/gobind
endef

lanternsdk-android.aar: $(SOURCES)
	@$(call build-tags) && \
	$(call prep-for-mobile) && \
	echo "Running gomobile with `which gomobile` version `GO111MODULE=off gomobile version` ..." && \
	GO111MODULE=off gomobile bind -o=lanternsdk-android.aar -target=android -tags='headless publicsdk' -ldflags="$$EXTRA_LDFLAGS -s -w" github.com/getlantern/flashlight/lanternsdk

# we build the LanternSDK.framework in two steps to use XCFramework
# See https://stackoverflow.com/questions/63942997/generate-xcframework-file-with-gomobile
Lanternsdk.xcframework: $(SOURCES)
	@$(call build-tags) && \
	$(call prep-for-mobile) && \
	echo "Running gomobile with `which gomobile` version `GO111MODULE=off gomobile version` ..." && \
	GO111MODULE=off gomobile bind -o=Lanternsdk.xcframework -target=ios -tags='headless publicsdk' -ldflags="$$EXTRA_LDFLAGS -s -w" github.com/getlantern/flashlight/lanternsdk

clean:
	rm -rf lanternsdk-android.aar Lanternsdk.xcframework vendor
