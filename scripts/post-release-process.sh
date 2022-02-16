#!/bin/sh
IMG=k8ssandra/cass-operator:latest

# Add new ## unreleased after the tagging (post-release-process.sh)
# Modify Makefile for the next VERSION in line

# Return config/manager/kustomization.yaml to :latest
cd config/manager && $KUSTOMIZE edit set image controller=$IMG && cd -

# Return config/manager/image_config.yaml to :latest
LOG_IMG=k8ssandra/system-logger:latest yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml
