#!/usr/bin/env bash

function die() {
  echo $*
  exit 1
}

#git add cloud.yaml || die "could not add file to git?"
#git commit -m "latest cloud.yaml file" || die "Could not commit cloud.yaml file"
#git push origin master || die "Could not push cloud.yaml"

#echo "Updating template"
#./certstotemplate.py -t cloud.yaml.tmpl -o cloud.yaml || die "Could not create new template"

echo "Adding cloud.yaml to s3"
s3cmd put -P cloud.yaml s3://lantern_config || die "Could not upload cloud.yaml to s3"

echo "File updated on s3"
