# Lantern [![wercker status](https://app.wercker.com/status/51826d53d0eeedd6efce16085874d82c/s/devel "wercker status")](https://app.wercker.com/project/byKey/51826d53d0eeedd6efce16085874d82c) [![Coverage Status](https://coveralls.io/repos/github/getlantern/flashlight/badge.svg?branch=HEAD&t=C4SaZX)](https://coveralls.io/github/getlantern/flashlight?branch=HEAD)

### Prerequisites

* [Go 1.7](https://golang.org/dl/) is the minimum supported version of Go
* [Dep](https://golang.github.io/dep/) On OSX and Linux: `go get -u github.com/golang/dep/cmd/dep`
* [GNU Make](https://www.gnu.org/software/make/) if you want to use the Makefile
* Force git to use ssh instead of https by running
  `git config --global url."git@github.com:".insteadOf "https://github.com/"`

This repo contains the core Lantern logic as well as the Lantern desktop
program.

To build using vendored dependencies: `make vendor lantern`

To build using your gopath: `make novendor lantern`

After running `make vendor` or `make novendor`, you don't need to run them again
unless you want to switch to/from using vendored dependencies.  After that, you
can just use `make`.

You can also just build using the usual `go install` etc.

### Updating Icons

The icons used for the system tray are stored in `icons_src`. To apply changes
to the icons, make your updates in the `icons_src` folder and then run
`make update-icons`.

### Forcing Ad Swapping

By default, only a small percentage of web ads are swapped and only if the user
is free (not pro), making it difficult to test ad swapping. However, you can
force 100% of eligible ads to be swapped even when pro by setting the
environment variable `FORCEADS=true`.

### Running CI Locally

You can run CI locally with the following:

```
brew tap wercker/wercker
brew install wercker-cli
wercker build
```
