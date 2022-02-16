#!/bin/sh
IMG=k8ssandra/cass-operator:latest
LOG_IMG=k8ssandra/system-logger:latest

# Add new ## unreleased after the tagging (post-release-process.sh)
# Modify Makefile for the next VERSION in line

# Return config/manager/kustomization.yaml to :latest
cd config/manager && bin/kustomize edit set image controller=$(IMG)

# Return config/manager/image_config.yaml to :latest
yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml
