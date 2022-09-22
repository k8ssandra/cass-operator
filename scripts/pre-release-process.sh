#!/bin/sh

if [ "$#" -ne 2 ]; then
    echo "Usage: scripts/pre-release-process.sh newTag previousTag"
    exit
fi

TAG=$1
PREVTAG=$2
#PREVTAG=$(git describe --abbrev=0 --tags)
IMG=artifacts.k8ssandra.io/k8ssandra/images/cass-operator:${TAG}

# Ensure kustomize is installed
make kustomize

KUSTOMIZE=$(pwd)/bin/kustomize

# Modify CHANGELOG automatically to match the tag
sed -i -e "s/## unreleased/## $TAG/" CHANGELOG.md

# Modify README to include proper installation refs
sed -i -e "s/$PREVTAG/$TAG/g" README.md

# Modify config/manager/kustomization.yaml to have proper newTag for cass-operator
cd config/manager && $KUSTOMIZE edit set image controller=$IMG && cd -

# Modify config/manager/image_config.yaml to have proper version for server-system-logger
LOG_IMG=artifacts.k8ssandra.io/k8ssandra/images/system-logger:${TAG} yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml
