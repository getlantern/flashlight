#!/usr/bin/env bash

set -e
git checkout devel
git pull

function refreshSubmodule {
  echo "Refreshing $1"
  local repo=$1
  git submodule update --remote --merge $repo
  git add $repo
  git commit -m "update to latest $repo master" && git push origin devel
  echo "Refreshed $repo"
}

function refreshSubmodules {
  rm -rf src/github.com/getlantern/lantern-desktop
  rm -rf android-beam
  rm -rf android-lantern
  git submodule update --init --recursive
  refreshSubmodule src/github.com/getlantern/lantern-desktop
  refreshSubmodule android-beam
  refreshSubmodule android-lantern
  echo "Refreshed all submodules"
}

function refreshGlobalConfig {
  echo "Refreshing global config"
  cd src/github.com/getlantern/flashlight/genconfig
  ./genglobal.bash
  cd -
  git submodule update --init
}

function pullTranslations {
  APP=lantern make android-pull-translations
  APP=beam make android-pull-translations
  echo "Pulled all translations"
}

refreshSubmodules
refreshGlobalConfig
pullTranslations
