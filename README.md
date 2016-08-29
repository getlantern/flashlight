# Lantern

### Prerequisites

* [Go 1.7](https://golang.org/dl/) is the minimum supported version of Go
* * [GNU Make](https://www.gnu.org/software/make/) if you want to use the Makefile

This repo contains the core Lantern logic as well as the Lantern desktop
program.

To build using vendored dependencies: `make vendor lantern`

To build using your gopath: `make novendor lantern`

After running `make vendor` or `make novendor`, you don't need to run them again
unless you want to switch to/from using vendored dependencies.  After that, you
can just use `make`.

You can also just build using the usual `go install` etc.
