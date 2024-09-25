DISABLE_PORT_RANDOMIZATION ?=

SHELL := /bin/bash
PROTO_SOURCES = $(shell find . -name '*.proto')
GENERATED_PROTO_SOURCES = $(shell echo "$(PROTO_SOURCES)" | sed 's/\.proto/\.pb\.go/g')
SOURCES = $(GENERATED_PROTO_SOURCES) $(shell find . -name '*[^_test].go')

REVISION_DATE := $(shell git log -1 --pretty=format:%ad --date=format:%Y%m%d.%H%M%S)
BUILD_DATE := $(shell date -u +%Y%m%d.%H%M%S)

VERSION ?= $$VERSION
LDFLAGS := -s -w -X github.com/getlantern/flashlight/common.RevisionDate=$(REVISION_DATE) -X github.com/getlantern/flashlight/common.BuildDate=$(BUILD_DATE) -X github.com/getlantern/flashlight/common.CompileTimePackageVersion=$(VERSION)

%.pb.go: %.proto
	protoc --go_out=. --go_opt=paths=source_relative $<

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

%.pb.go: %.proto
	go build -o build/protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go
	protoc --go_out=. --plugin=build/protoc-gen-go --go_opt=paths=source_relative $<

clean:
	rm -rf .gomobilecache

.PHONY: install-githooks
install-githooks:
	cp githooks/* .git/hooks/


.PHONY: build-genconfig
build-genconfig:
	go build -o ./genconfig/genconfig ./genconfig/genconfig.go
