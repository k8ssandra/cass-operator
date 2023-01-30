#!/bin/sh

if [ "$#" -ne 1 ]; then
    echo "Usage: scripts/pre-release-process.sh newTag"
    exit
fi

TAG=$1
#PREVTAG=$2
PREVTAG=$(git describe --abbrev=0 --tags)
IMG=k8ssandra/cass-operator:${TAG}

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
LOG_IMG=k8ssandra/system-logger:${TAG} yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml

# Now add everything and create a commit + tag
git add CHANGELOG.md
git add README.md
git add config/manager/kustomization.yaml
git add config/manager/image_config.yaml

git commit -m "Release $TAG"
git tag -a $TAG -m "Release $TAG"
