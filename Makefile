DISABLE_PORT_RANDOMIZATION ?=

SHELL := /bin/bash
SOURCES := $(shell find . -name '*[^_test].go')

REVISION_DATE := $(shell git log -1 --pretty=format:%ad --date=format:%Y%m%d.%H%M%S)
BUILD_DATE := $(shell date -u +%Y%m%d.%H%M%S)

VERSION ?= $$VERSION
LDFLAGS := -s -w -X github.com/getlantern/flashlight/common.RevisionDate=$(REVISION_DATE) -X github.com/getlantern/flashlight/common.BuildDate=$(BUILD_DATE) -X github.com/getlantern/flashlight/common.CompileTimePackageVersion=$(VERSION)

test-and-cover: $(SOURCES)
	@echo "mode: count" > profile.cov && \
	TP=$$(go list ./...) && \
	CP=$$(echo $$TP | tr ' ', ',') && \
	set -x && \
	for pkg in $$TP; do \
		go test -race -v -tags="headless" -covermode=atomic -coverprofile=profile_tmp.cov -coverpkg "$$CP" $$pkg || exit 1; \
		tail -n +2 profile_tmp.cov >> profile.cov; \
	done

test:
	go test -failfast -race -v -tags="headless" ./...

define prep-for-mobile
	go env -w 'GOPRIVATE=github.com/getlantern/*'
	go install golang.org/x/mobile/cmd/gomobile
	gomobile init
endef

lanternsdk-android.aar: $(SOURCES)
	$(call prep-for-mobile) && \
	echo "LDFLAGS $(LDFLAGS)" && \
	echo "Running gomobile with `which gomobile` version `gomobile version` ..." && \
	gomobile bind -cache `pwd`/.gomobilecache -o=lanternsdk-android.aar -target=android -tags='headless publicsdk' -ldflags="$(LDFLAGS)" github.com/getlantern/flashlight/lanternsdk

# we build the LanternSDK.framework in two steps to use XCFramework
# See https://stackoverflow.com/questions/63942997/generate-xcframework-file-with-gomobile
Lanternsdk.xcframework: $(SOURCES)
	@$(call prep-for-mobile) && \
	echo "Running gomobile with `which gomobile` version `gomobile version` ..." && \
	gomobile bind -cache `pwd`/.gomobilecache -o=Lanternsdk.xcframework -target=ios -tags='headless publicsdk' -ldflags="$(LDFLAGS)" github.com/getlantern/flashlight/lanternsdk

clean:
	rm -rf .gomobilecache lanternsdk-android.aar Lanternsdk.xcframework
