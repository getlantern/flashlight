#!/usr/bin/env bash

ip=$1

source $(dirname "$0")/deployUtils.bash

if [ $# -ne "1" ]
then
    die_without_cleanup "$0: Received $# args... IP required"
fi

check_is_proxy_server

echo "Disabling auto-update on $ip"
ssh lantern@$ip -t "sudo crontab -l | perl -p -e 's/^(.*update_proxy.bash.*)/#\1/g' | sudo crontab -" || die "Could not disable auto-updates"

echo "Uploading http-proxy-lantern"
scp dist-bin/http-proxy lantern@$ip:http-proxy.tmp || die "Could not copy binary"

echo "Stopping http-proxy-lantern to allow replacing binary"
ssh lantern@$ip -t "sudo service http-proxy stop" 

echo "Replacing binary"
ssh lantern@$ip -t "sudo cp /home/lantern/http-proxy.tmp /home/lantern/http-proxy" || cleanup "Could not replace binary"

setcap_proxy

cleanup
