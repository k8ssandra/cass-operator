#!/bin/sh

if [ "$#" -ne 4 ]; then
    echo "Usage: scripts/release-certified-bundles.sh version sha256:<cass-operator-sha256> sha256:<system-logger-sha256> sha256:<k8ssandra-client-sha256>"
    echo "Script assumes you are in the correct branch / tag and that community-operators repository"
    echo "has been checked out to ../community-operators/"
    exit
fi

VERSION=$1
SHA=$2
SYSTEM_LOGGER_SHA=$3
CLIENT_SHA=$4
# TODO Add certified-operators-marketplace ?
TARGET_DIRS=(certified-operators)

# Checkout tag
git checkout v$VERSION

yq -i '.images.system-logger = "registry.connect.redhat.com/datastax/system-logger@"' config/manager/image_config.yaml
#yq -i '.images.k8ssandra-client = "registry.connect.redhat.com/datastax/k8ssandra-client@"' config/manager/image_config.yaml
SYSTEM_LOGGER_SHA=$SYSTEM_LOGGER_SHA yq -i '.images.system-logger += env(SYSTEM_LOGGER_SHA)' config/manager/image_config.yaml

# Create bundle
make VERSION=$VERSION IMG=registry.connect.redhat.com/datastax/cass-operator@$SHA bundle

# Add relatedImages
yq -i '.spec.relatedImages = []' bundle/manifests/cass-operator.clusterserviceversion.yaml
yq -i '.spec.relatedImages += {"name": "cass-operator", "image": "registry.connect.redhat.com/datastax/cass-operator@"}' bundle/manifests/cass-operator.clusterserviceversion.yaml
yq -i '.spec.relatedImages += {"name": "system-logger", "image": "registry.connect.redhat.com/datastax/system-logger@"}' bundle/manifests/cass-operator.clusterserviceversion.yaml
#yq -i '.spec.relatedImages += {"name": "k8ssandra-client", "image": "registry.connect.redhat.com/datastax/k8ssandra-client@"}' bundle/manifests/cass-operator.clusterserviceversion.yaml
SHA=$SHA yq -i '.spec.relatedImages[0].image += env(SHA)' bundle/manifests/cass-operator.clusterserviceversion.yaml
SYSTEM_LOGGER_SHA=$SYSTEM_LOGGER_SHA yq -i '.spec.relatedImages[1].image += env(SYSTEM_LOGGER_SHA)' bundle/manifests/cass-operator.clusterserviceversion.yaml
CLIENT_SHA=$CLIENT_SHA yq -i '.spec.relatedImages[2].image += env(CLIENT_SHA)' bundle/manifests/cass-operator.clusterserviceversion.yaml

for dir in "${TARGET_DIRS[@]}"
do
    TARGET_DIR=../$dir/operators/cass-operator/v$VERSION
    mkdir $TARGET_DIR
    cp -R bundle/* $TARGET_DIR

    cd $TARGET_DIR
    git checkout -b cass-operator-$VERSION main
    git add .
    git commit -s -am "operator cass-operator (v${VERSION})"
    cd -
done
