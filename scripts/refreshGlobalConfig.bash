#!/usr/bin/env bash

function die() {
  echo "$@"
  exit 1
}

set -e

curl https://globalconfig.flashlightproxy.com/global.yaml.gz | gunzip >> yaml-temp

curdir=`pwd`
echo $curdir
echo 'package generated' > ./config/generated/embeddedGlobal.go && \
echo '' >> ./config/generated/embeddedGlobal.go && \
echo 'var GlobalConfig = []byte(`' >> ./config/generated/embeddedGlobal.go && \
cat yaml-temp >> ./config/generated/embeddedGlobal.go && \
echo '`)' >> ./config/generated/embeddedGlobal.go || die "Unable to generate embeddedGlobal.go"

rm yaml-temp

cd ./config || die "Could not change directories"
go test -run TestGlobal || die "Global test failed"

#git add generated/embeddedGlobal.go || die "Could not add resources?"
#git commit -m "pushing auto-generated embedded global config" 

echo "Finished generating resources and added embeddedGlobal.go."
