DISABLE_PORT_RANDOMIZATION ?=
GLIDE_BIN    ?= $(shell which glide)

SHELL := /bin/bash
SOURCES := $(shell find . -name '*[^_test].go')

.PHONY: lantern

BUILD_RACE := '-race'

ifeq ($(OS),Windows_NT)
	  # Race detection is not supported by Go Windows 386, so disable it. The -x
		# is just a hack to allow us to pass something in place of -race below.
		BUILD_RACE = '-x'
endif

define build-tags
	BUILD_TAGS="$(BUILD_TAGS)" && \
	EXTRA_LDFLAGS="" && \
	if [[ ! -z "$$VERSION" ]]; then \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.CompileTimePackageVersion=$$VERSION"; \
	else \
		echo "** VERSION was not set, using default version. This is OK while in development."; \
	fi && \
	if [[ ! -z "$$HEADLESS" ]]; then \
		BUILD_TAGS="$$BUILD_TAGS headless"; \
	fi && \
	if [[ ! -z "$$DISABLE_PORT_RANDOMIZATION" ]]; then \
		BUILD_TAGS="$$BUILD_TAGS disableresourcerandomization"; \
	fi && \
	if [[ ! -z "$$STAGING" ]]; then \
		BUILD_TAGS="$$BUILD_TAGS staging"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/common.StagingMode=$$STAGING"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/lantern.StagingMode=$$STAGING"; \
	fi && \
	BUILD_TAGS=$$(echo $$BUILD_TAGS | xargs) && echo "Build tags: $$BUILD_TAGS" && \
	EXTRA_LDFLAGS=$$(echo $$EXTRA_LDFLAGS | xargs) && echo "Extra ldflags: $$EXTRA_LDFLAGS"
endef

.PHONY: require-glide vendor novendor

lantern: $(SOURCES)
	@$(call build-tags) && \
	CGO_ENABLED=1 go build $(BUILD_RACE) -o lantern -tags="$$BUILD_TAGS" -ldflags="$$EXTRA_LDFLAGS -s" github.com/getlantern/flashlight/main;

windowscli: $(SOURCES)
	@$(call build-tags) && \
	GOOS=windows GOARCH=386 CGO_ENABLED=1 go build -o lantern.exe -tags="$$BUILD_TAGS" -ldflags="$(LDFLAGS_NOSTRIP) $$EXTRA_LDFLAGS" github.com/getlantern/flashlight/main;

windowsgui: $(SOURCES)
	@$(call build-tags) && \
	GOOS=windows GOARCH=386 CGO_ENABLED=1 go build -a -o lantern.exe -tags="$$BUILD_TAGS" -ldflags="$(LDFLAGS_NOSTRIP) $$EXTRA_LDFLAGS -H=windowsgui" github.com/getlantern/flashlight/main;

linux: $(SOURCES)
	@$(call build-tags) && \
	HEADLESS=true GOOS=linux GOARCH=amd64 go build -o lantern-linux -tags="$$BUILD_TAGS headless" -ldflags="$$EXTRA_LDFLAGS" github.com/getlantern/flashlight/main;

# vendor installs vendored dependencies using Glide
vendor: require-glide
	@$(GLIDE_BIN) install

require-glide:
	@if [ "$(GLIDE_BIN)" = "" ]; then \
		echo 'Missing "glide" command. See https://github.com/Masterminds/glide' && exit 1; \
	fi

# novendor removes the vendor folder to allow building with whatever is on your
# GOPATH
novendor:
	@rm -Rf vendor

test-and-cover: $(SOURCES)
	@echo "mode: count" > profile.cov && \
	TP=$$(find . -name "*_test.go" -printf '%h\n' | grep  -v vendor | grep -v glide | sort -u) && \
	CP=$$(echo $$TP | tr ' ', ',') && \
	set -x && \
	for pkg in $$TP; do \
		go test -race -v -tags="headless" -covermode=atomic -coverprofile=profile_tmp.cov -coverpkg "$$CP" $$pkg || exit 1; \
		tail -n +2 profile_tmp.cov >> profile.cov; \
	done

test: $(SOURCES)
	@TP=$$(glide novendor -x) && \
	go test -race -v -tags="headless" $$TP || exit 1; \

clean:
	rm -f lantern
