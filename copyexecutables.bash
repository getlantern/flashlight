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

lantern=$1/install
codesign -s "Developer ID Application: Brave New Software Project, Inc" -f flashlight_darwin_amd64 || die "Could not code sign?"

echo "Copying executables to $1"

cp flashlight_darwin_amd64 $lantern/osx/pt/flashlight/ || die "Could not copy darwin"
cp flashlight_windows_386.exe $lantern/win/pt/flashlight/ || die "Could not copy windows"
cp flashlight_linux_386 $lantern/linux_x86_32/pt/flashlight/ || die "Could not copy linux 32"
cp flashlight_linux_amd64 $lantern/linux_x86_64/pt/flashlight/ || die "Could not copy linux 64"
