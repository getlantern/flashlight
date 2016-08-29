GLIDE_BIN    ?= $(shell which glide)
BUILD_DIR    ?= bin

SHELL := /bin/bash
SOURCES := $(shell find . -name '*[^_test].go')

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
		EXTRA_LDFLAGS="-X github.com/getlantern/lantern.compileTimePackageVersion=$$VERSION -X github.com/getlantern/flashlight.compileTimePackageVersion=$$VERSION"; \
	else \
		echo "** VERSION was not set, using default version. This is OK while in development."; \
	fi && \
	if [[ ! -z "$$HEADLESS" ]]; then \
		BUILD_TAGS="$$BUILD_TAGS headless"; \
	fi && \
	if [[ ! -z "$$STAGING" ]]; then \
		BUILD_TAGS="$$BUILD_TAGS staging"; \
		EXTRA_LDFLAGS="$$EXTRA_LDFLAGS -X github.com/getlantern/flashlight/app.stagingMode=$$STAGING -X github.com/getlantern/lantern.stagingMode=$$STAGING"; \
	fi && \
	BUILD_TAGS=$$(echo $$BUILD_TAGS | xargs) && echo "Build tags: $$BUILD_TAGS" && \
	EXTRA_LDFLAGS=$$(echo $$EXTRA_LDFLAGS | xargs) && echo "Extra ldflags: $$EXTRA_LDFLAGS"
endef

.PHONY: require-glide vendor novendor

lantern: $(SOURCES)
	@$(call build-tags) && \
	CGO_ENABLED=1 go build $(BUILD_RACE) -o lantern -tags="$$BUILD_TAGS" -ldflags="$(LDFLAGS_NOSTRIP) $$EXTRA_LDFLAGS" github.com/getlantern/flashlight/main;

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

clean:
	rm lantern
