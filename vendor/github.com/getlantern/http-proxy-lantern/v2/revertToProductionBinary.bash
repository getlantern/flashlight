#!/usr/bin/env bash

ip=$1

source $(dirname "$0")/deployUtils.bash

if [ $# -ne "1" ]
then
    die_without_cleanup "$0: Received $# args... IP required"
fi

check_is_proxy_server

echo "Enabling auto-update on $ip"
ssh lantern@$ip -t "sudo crontab -l | perl -p -e 's/^#(.*update_proxy.bash.*)/\1/g' | sudo crontab -" || die "Could not reenable auto-updates"

echo "Stopping http-proxy-lantern to allow reverting binary"
ssh lantern@$ip -t "sudo service http-proxy stop"

echo "Reverting binary"
ssh lantern@$ip -t "sudo cp /home/lantern/update/http-proxy /home/lantern/http-proxy" || die "Could not revert binary"

setcap_proxy

cleanup
