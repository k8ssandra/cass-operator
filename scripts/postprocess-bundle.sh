#!/bin/sh

# These labels are required to be in the bundle.Dockerfile, but can't be added by the operator-sdk automatically
cat <<EOF >> bundle.Dockerfile
# Certified Openshift required labels
LABEL com.redhat.openshift.versions="v4.5"
LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.delivery.backport=true
EOF

# This file is extra from creation process on config/manifests, should not be in the bundle itself
rm -f bundle/field-config_v1_configmap.yaml
