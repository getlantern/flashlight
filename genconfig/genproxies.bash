#!/usr/bin/env bash

function die() {
  echo $*
  exit 1
}

if [ -z "$LANTERN_AWS_PATH" ]; then
  echo "LANTERN_AWS_PATH is not set, defaults to $HOME/lantern_aws"
  lantern_aws_path=$HOME/lantern_aws
else
  lantern_aws_path=$LANTERN_AWS_PATH
fi

etc=$lantern_aws_path/etc
if [ ! -d "$etc" ]; then
  die "$etc doesn't exist or is not a directory"
fi

echo "Generating proxies..."
cd $etc
git checkout master || die "Could not checkout master?"
git pull || die "Could not pull latest code?"
git submodule update  || die "Could not update submodules?"
./fetchcfg.py sea > proxies.yaml || die "Could not fetch proxy in sea region?"
./fetchcfg.py sea >> proxies.yaml || die "Could not fetch proxy in sea region?"
./fetchcfg.py sea >> proxies.yaml || die "Could not fetch proxy in sea region?"
./fetchcfg.py sea >> proxies.yaml || die "Could not fetch proxy in sea region?"
./fetchcfg.py etc >> proxies.yaml || die "Could not fetch proxy in etc region?"
./fetchcfg.py etc >> proxies.yaml || die "Could not fetch proxy in etc region?"
cd -

echo 'package generated' > ../config/generated/embeddedProxies.go && \
echo '' >> ../config/generated/embeddedProxies.go && \
echo 'var EmbeddedProxies = []byte(`' >> ../config/generated/embeddedProxies.go && \
cat $etc/proxies.yaml >> ../config/generated/embeddedProxies.go && \
echo '`)' >> ../config/generated/embeddedProxies.go || die "Unable to generate embeddedProxies.go"

git add ../config/generated/embeddedProxies.go || die "Could not add proxies?"

echo "Finished generating proxies and added ../config/generated/embeddedProxies.go. Please simply commit that file after confirming the process seemed to have correctly generatated everything -- check lantern.yaml in particular, but no need to check that in"
