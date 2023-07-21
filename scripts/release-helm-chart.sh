#!/bin/sh

# This script assumes k8ssandra is checked out at ../k8ssandra and is checked out at main
if [ "$#" -ne 1 ]; then
    echo "Usage: scripts/release-helm-chart.sh version"
    echo "Script assumes you are in the correct branch / tag and that k8ssandra repository"
    echo "has been checked out to ../k8ssandra/"
    exit
fi

VERSION=$1
CHART_HOME=../k8ssandra/charts/cass-operator
CRD_TARGET_PATH=$CHART_HOME/crds
TEMPLATE_HOME=$CHART_HOME/templates

# Checkout tag
git checkout v$VERSION

# Create CRDs
kustomize build config/crd  --output $CRD_TARGET_PATH

# Rename generated CRDs to shorter format
for f in $CRD_TARGET_PATH/*; do
    TARGET_FILENAME=$(yq '.spec.names.plural' $f).yaml
    mv $f $CRD_TARGET_PATH/$TARGET_FILENAME
done

# TODO Update all the necessary fields also (Chart.yaml, values.yaml, configmap.yaml)
