#!/usr/bin/env bash

function die() {
  echo $*
  exit 1
}

lantern=$HOME/lantern/install
cp $GOPATH/bin/flashlight-xc/snapshot/darwin_amd64/flashlight $lantern/osx/pt/flashlight/ || die "Could not copy darwin"
cp $GOPATH/bin/flashlight-xc/snapshot/windows_386/flashlight.exe $lantern/win/pt/flashlight/ || die "Could not copy windows"
cp $GOPATH/bin/flashlight-xc/snapshot/linux_386/flashlight $lantern/linux_x86_32/pt/flashlight/ || die "Could not copy linux 32"
cp $GOPATH/bin/flashlight-xc/snapshot/linux_amd64/flashlight $lantern/linux_x86_64/pt/flashlight/ || die "Could not copy linux 64"
