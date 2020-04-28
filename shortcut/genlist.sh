#! /usr/bin/env bash

function die() {
  echo "$@"
  exit 1
}

function gen-list-for-country() {
  grep $1 GeoLite2-Country-Blocks-IPv4.csv | cut -d "," -f 1 > resources/$2_ipv4.txt && \
  grep $1 GeoLite2-Country-Blocks-IPv6.csv | cut -d "," -f 1 > resources/$2_ipv6.txt && \
  if [[ $2 == "ir" ]]; then \
    grep -v -e "^#" resources/default_ipv4_$2.txt >> resources/$2_ipv4.txt; \
  else
    cat resources/default_ipv4.txt >> resources/$2_ipv4.txt; \
  fi && \
  cat resources/default_ipv6.txt >> resources/$2_ipv6.txt
}

which go-bindata >/dev/null || die 'Missing command "go-bindata". See https://github.com/jteeuwen/go-bindata.'
[ ! -z "$MAXMIND_LICENSE_KEY" ] || die 'Missing envvar "MAXMIND_LICENSE_KEY".'
mkdir -p resources && \
curl "https://download.maxmind.com/app/geoip_download?license_key=$MAXMIND_LICENSE_KEY&edition_id=GeoLite2-Country-CSV&suffix=zip" > geoip2.zip && \
unzip -j geoip2.zip && \
# See resources/GeoLite2-Country-Locations-en.csv for the mapping of id to country code
gen-list-for-country 1814991 cn && \
gen-list-for-country 130758 ir && \
gen-list-for-country 290557 ae && \
rm geoip2.zip && \
rm *.csv  && \
rm *.txt  && \
go-bindata -nomemcopy -nocompress -pkg shortcut -o resources.go resources
