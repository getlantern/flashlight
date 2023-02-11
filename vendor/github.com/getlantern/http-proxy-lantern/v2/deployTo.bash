#!/usr/bin/env bash

ip=$1

source $(dirname "$0")/deployUtils.bash

"${VERSION:?VERSION required}"

rm dist/*

echo "Building http-proxy-lantern for $ip"
make distnochange || die_without_cleanup "Could not make dist for http proxy"

./onlyDeployTo.bash $ip
