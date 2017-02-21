#!/usr/bin/env bash

function die() {
  echo $*
  exit 1
}

if [ $# -ne "1" ]
then
    die "$0: IP required, as in $0 1.1.1.1"
fi
ip=$1
tok=`ssh lantern@$ip "cat auth_token.txt"`

./lantern --force-proxy-addr=$ip --force-auth-token=$tok
