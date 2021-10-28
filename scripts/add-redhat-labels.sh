#!/bin/sh

cat <<EOF >> bundle.Dockerfile

# Certified Openshift required labels
LABEL com.redhat.openshift.versions="v4.5"
LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.delivery.backport=true
EOF
