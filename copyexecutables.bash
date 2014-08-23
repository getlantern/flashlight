#!/usr/bin/env bash

function die() {
  echo $*
  exit 1
}

if [ $# -lt "1" ]
then
    die "$0: Path to lantern required"
fi

# Sign while we're at it...

xcbase=$GOPATH/bin/flashlight-xc/snapshot
lantern=$1/install
codesign -s "Developer ID Application: Brave New Software Project, Inc" -f $xcbase/darwin_amd64/flashlight || die "Could not code sign?"

echo "Copying executables to $1"

cp $xcbase/darwin_amd64/flashlight $lantern/osx/pt/flashlight/ || die "Could not copy darwin"
cp $xcbase/windows_386/flashlight.exe $lantern/win/pt/flashlight/ || die "Could not copy windows"
cp $xcbase/linux_386/flashlight $lantern/linux_x86_32/pt/flashlight/ || die "Could not copy linux 32"
cp $xcbase/linux_amd64/flashlight $lantern/linux_x86_64/pt/flashlight/ || die "Could not copy linux 64"
