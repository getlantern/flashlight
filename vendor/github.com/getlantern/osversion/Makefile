.PHONY: run-test-cmd
run-test-cmd:
	go run ./cmd/main.go

# -androidapi should be specified, else gomobile (as of 2023-01-09) will have a
#  hard time detecting the correct NDK version
.PHONY: build-test-android-app
build-test-android-app:
	go install golang.org/x/mobile/cmd/gomobile@latest
	gomobile init
	gomobile build \
		-androidapi=19 \
		github.com/getlantern/osversion/cmd

.PHONY: clean
clean:
	rm -rf cmd.apk
