#!/bin/sh

if [ "$#" -ne 2 ]; then
    echo "Usage: scripts/release-certified-bundles.sh version sha256:<sha256>"
    echo "Script assumes you are in the correct branch / tag and that community-operators repository"
    echo "has been checked out to ../community-operators/"
    exit
fi

VERSION=$1
SHA=$2
# TODO Add certified-operators-marketplace
TARGET_DIRS=(certified-operators)
SYSTEM_LOGGER_SHA=sha256:33e75d0c78a277cdc37be24f2b116cade0d9b7dc7249610cdf9bf0705c8a040e

# Checkout tag
git checkout v$VERSION

yq -i '.images.system-logger = "registry.connect.redhat.com/datastax/system-logger@"' config/manager/image_config.yaml
SYSTEM_LOGGER_SHA=$SYSTEM_LOGGER_SHA yq -i '.images.system-logger += env(SYSTEM_LOGGER_SHA)' config/manager/image_config.yaml

# Create bundle
make VERSION=$VERSION IMG=registry.connect.redhat.com/datastax/cass-operator@$SHA bundle

# Add relatedImages
yq -i '.spec.relatedImages = []' bundle/manifests/cass-operator.clusterserviceversion.yaml
yq -i '.spec.relatedImages += {"name": "cass-operator", "image": "registry.connect.redhat.com/datastax/cass-operator@"}' bundle/manifests/cass-operator.clusterserviceversion.yaml
yq -i '.spec.relatedImages += {"name": "system-logger", "image": "registry.connect.redhat.com/datastax/system-logger@"}' bundle/manifests/cass-operator.clusterserviceversion.yaml
SHA=$SHA yq -i '.spec.relatedImages[0].image += env(SHA)' bundle/manifests/cass-operator.clusterserviceversion.yaml
SYSTEM_LOGGER_SHA=$SYSTEM_LOGGER_SHA yq -i '.spec.relatedImages[1].image += env(SYSTEM_LOGGER_SHA)' bundle/manifests/cass-operator.clusterserviceversion.yaml

for dir in "${TARGET_DIRS[@]}"
do
    TARGET_DIR=../$dir/operators/cass-operator/$VERSION
    mkdir $TARGET_DIR
    cp -R bundle/* $TARGET_DIR

    cd $TARGET_DIR
    git checkout -b cass-operator-$VERSION main
    git add .
    git commit -s -am "cass-operator v${VERSION}"
    cd -
done
