#!/bin/sh
IMG=k8ssandra/cass-operator:latest
KUSTOMIZE=$(pwd)/bin/kustomize

# Add new ## unreleased after the tagging (post-release-process.sh)
gawk -i inplace  '/##/ && ++c==1 { print "## unreleased\n"; print; next }1' CHANGELOG.md

# Return config/manager/kustomization.yaml to :latest
cd config/manager && $KUSTOMIZE edit set image controller=$IMG && cd -

# Return config/manager/image_config.yaml to :latest
LOG_IMG=k8ssandra/system-logger:latest yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml

# Commit to git
NEXT_VERSION=$(gawk 'match($0, /^VERSION \?= /) { print substr($0, RLENGTH+1)}' Makefile)

git add Makefile
git add CHANGELOG.md
git add config/manager/kustomization.yaml
git add config/manager/image_config.yaml
