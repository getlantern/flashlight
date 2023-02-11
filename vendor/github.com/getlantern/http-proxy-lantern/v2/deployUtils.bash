#!/usr/bin/env bash

function cleanup() {
  echo "Starting http-proxy-lantern"
  ssh lantern@$ip -t "sudo service http-proxy start" || die_without_cleanup "Could not start http-proxy"
}

function die() {
  echo "$@"
  cleanup
  exit 1
}

function die_without_cleanup() {
  echo "$@"
  exit 1
}

function check_is_proxy_server() {
  echo "Ensuring this is actually a http-proxy server"
  ssh lantern@$ip -t "sudo systemctl cat http-proxy >/dev/null 2>&1" || die_without_cleanup "http-proxy service does not exist on this server. It might not actually be a http-proxy-lantern server"
}

function setcap_proxy() {
  echo "Calling setcap on http-proxy"
  ssh lantern@$ip -t "sudo setcap 'cap_net_raw+eip cap_net_admin+eip cap_net_bind_service+ep' /home/lantern/http-proxy" || die "Error calling setcap on http-proxy"
}
