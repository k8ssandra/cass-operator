#!/bin/sh
if [[ ! $0 == scripts/* ]]; then
    echo "This script must be run from the root directory"
    exit 1
fi

IMG=k8ssandra/cass-operator:latest
KUSTOMIZE=$(pwd)/bin/kustomize

# Add new ## unreleased after the tagging (post-release-process.sh)
gawk -i inplace  '/##/ && ++c==1 { print "## unreleased\n"; print; next }1' CHANGELOG.md

# Modify Makefile for the next VERSION in line
scripts/update-makefile-version.sh

# Return config/manager/kustomization.yaml to :latest
cd config/manager && $KUSTOMIZE edit set image controller=$IMG && cd -

# Return config/manager/image_config.yaml to :latest
LOG_IMG=k8ssandra/system-logger:latest yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml

# Remove cr.k8ssandra.io prefixes
yq eval -i '.images.k8ssandra-client |= sub("cr.k8ssandra.io/", "")' config/manager/image_config.yaml
yq eval -i '.defaults.cassandra.repository |= sub("cr.k8ssandra.io/", "")' config/manager/image_config.yaml

# Remove cr.dstx.io prefixes
yq eval -i '.images.config-builder |= sub("cr.dtsx.io/", "")' config/manager/image_config.yaml
yq eval -i '.defaults.dse.repository |= sub("cr.dtsx.io/", "")' config/manager/image_config.yaml



# Commit to git
NEXT_VERSION=$(gawk 'match($0, /^VERSION \?= /) { print substr($0, RLENGTH+1)}' Makefile)

git add Makefile
git add CHANGELOG.md
git add config/manager/kustomization.yaml
git add config/manager/image_config.yaml

git commit -m "Prepare for next version $NEXT_VERSION"
