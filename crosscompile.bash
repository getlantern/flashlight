#!/bin/bash

VERSION="`git describe --abbrev=0 --tags`"
goxc -build-ldflags="-w -X main.version $VERSION" -bc="linux,386 linux,amd64 windows,386 darwin" validate compile
