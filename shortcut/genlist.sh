#! /usr/bin/env bash

mkdir -p resources && \
(which go-bindata >/dev/null || (echo 'Missing command "go-bindata". Sett https://github.com/jteeuwen/go-bindata.' && exit 1)) && \
(curl 'http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest' | grep CN | \
tee >(grep ipv4 | awk -F\| '{ printf("%s/%d\n", $4, 32-log($5)/log(2)) }' > resources/cn_ipv4.txt) | \
grep ipv6 | awk -F\| '{ printf("%s/%d\n", $4, $5) }' > resources/cn_ipv6.txt) && \
cat resources/default_ipv4.txt >> resources/cn_ipv4.txt && \
cat resources/default_ipv6.txt >> resources/cn_ipv6.txt && \
go-bindata -nomemcopy -nocompress -pkg shortcut -o resources.go resources
