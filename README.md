# Lantern

This repo contains the core Lantern logic as well as the Lantern desktop
program.

To build using vendored dependencies: `make vendor lantern`

To build using your gopath: `make novendor lantern`

After running `make vendor` or `make novendor`, you don't need to run them again
unless you want to switch to/from using vendored dependencies.  After that, you
can just use `make`.
