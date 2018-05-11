#!/usr/bin/env bash

function die() {
  echo "$@"
  exit 1
}

curl https://globalconfig.flashlightproxy.com/global.yaml.gz | gunzip >> yaml-temp

echo 'package generated' > ../config/generated/embeddedGlobal.go && \
echo '' >> ../config/generated/embeddedGlobal.go && \
echo 'var GlobalConfig = []byte(`' >> ../config/generated/embeddedGlobal.go && \
cat yaml-temp >> ../config/generated/embeddedGlobal.go && \
echo '`)' >> ../config/generated/embeddedGlobal.go || die "Unable to generate embeddedGlobal.go"

rm yaml-temp

git add ../config/generated/embeddedGlobal.go || die "Could not add resources?"

echo "Finished generating resources and added ../config/generated/embeddedGlobal.go. Please simply commit that file after manually viewing it to make sure it looks sane"
