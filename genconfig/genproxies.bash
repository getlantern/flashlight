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

# As of this writing these are the tracks we're using to serve free proxies,
# which are also the ones we keep populated with baked-in proxies ready to be
# fetched.
./fetch_bakedin_config.py doams3-baked-in >> proxies.yaml || die "Could not fetch proxy in doams3-baked-in track?"
./fetch_bakedin_config.py doblr1-baked-in >> proxies.yaml || die "Could not fetch proxy in doblr1-baked-in track?"
./fetch_bakedin_config.py dofra1-baked-in >> proxies.yaml || die "Could not fetch proxy in dofra1-baked-in track?"
./fetch_bakedin_config.py dolon1-baked-in >> proxies.yaml || die "Could not fetch proxy in dolon1-baked-in track?"
./fetch_bakedin_config.py donyc3-baked-in >> proxies.yaml || die "Could not fetch proxy in donyc3-baked-in track?"
./fetch_bakedin_config.py dosgp1-baked-in >> proxies.yaml || die "Could not fetch proxy in dosgp1-baked-in track?"
./fetch_bakedin_config.py vllos1-baked-in >> proxies.yaml || die "Could not fetch proxy in vllos1-baked-in track?"
./fetch_bakedin_config.py vlsgp1-baked-in >> proxies.yaml || die "Could not fetch proxy in vlsgp1-baked-in track?"

cd -

echo 'package generated' > ../config/generated/embeddedProxies.go && \
echo '' >> ../config/generated/embeddedProxies.go && \
echo 'var EmbeddedProxies = []byte(`' >> ../config/generated/embeddedProxies.go && \
cat $etc/proxies.yaml >> ../config/generated/embeddedProxies.go && \
echo '`)' >> ../config/generated/embeddedProxies.go || die "Unable to generate embeddedProxies.go"

git add ../config/generated/embeddedProxies.go || die "Could not add proxies?"

echo "Finished generating proxies and added ../config/generated/embeddedProxies.go. Please simply commit that file after confirming the process seemed to have correctly generatated everything -- check lantern.yaml in particular, but no need to check that in"
