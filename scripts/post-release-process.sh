#!/bin/sh
IMG=us-docker.pkg.dev/k8ssandra/images/cass-operator:latest
KUSTOMIZE=$(pwd)/bin/kustomize

# Add new ## unreleased after the tagging (post-release-process.sh)
gawk -i inplace  '/##/ && ++c==1 { print "## unreleased\n"; print; next }1' CHANGELOG.md

# Modify Makefile for the next VERSION in line


# Return config/manager/kustomization.yaml to :latest
cd config/manager && $KUSTOMIZE edit set image controller=$IMG && cd -

# Return config/manager/image_config.yaml to :latest
LOG_IMG=us-docker.pkg.dev/k8ssandra/images/system-logger:latest yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml
