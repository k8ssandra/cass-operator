#!/bin/sh

TAG=$1
PREVTAG=$2
#PREVTAG=$(git describe --abbrev=0 --tags)
IMG=k8ssandra/cass-operator:$TAG
LOG_IMG=k8ssandra/system-logger:$TAG

# Modify CHANGELOG automatically to match the tag
sed -i -e "s/## unreleased/## $TAG/" CHANGELOG.md

# Modify README to include proper installation refs
sed -i -e "s/$PREVTAG/$TAG/g" README.md

# Modify config/manager/kustomization.yaml to have proper newTag for cass-operator
cd config/manager && bin/kustomize edit set image controller=$(IMG)

# Modify config/manager/image_config.yaml to have proper version for server-system-logger
yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml