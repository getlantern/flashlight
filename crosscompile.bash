#!/bin/bash

VERSION="`git describe --abbrev=0 --tags`"
BUILD_DATE="`date -u +%Y%m%d%.%H%M%S`"
goxc -build-ldflags="-w -X main.version $VERSION -X main.buildDate $BUILD_DATE" -bc="linux,386 linux,amd64 windows,386 darwin" validate compile
