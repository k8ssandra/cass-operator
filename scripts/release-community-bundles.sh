#!/bin/sh
if [[ ! $0 == scripts/* ]]; then
    echo "This script must be run from the root directory"
    exit 1
fi

if [ "$#" -ne 1 ]; then
    echo "Usage: scripts/release-community-bundles.sh version"
    echo "Script assumes you are in the correct branch / tag and that community-operators repository"
    echo "has been checked out to ../community-operators/"
    exit
fi

VERSION=$1
TARGET_DIRS=(community-operators community-operators-prod)

# Checkout tag
git checkout v$VERSION

# Create bundle
make VERSION=$VERSION REGISTRY=cr.k8ssandra.io bundle

# Modify package name to cass-operator-community
yq eval -i '.annotations."operators.operatorframework.io.bundle.package.v1" = "cass-operator-community"' bundle/metadata/annotations.yaml

for dir in "${TARGET_DIRS[@]}"
do
    TARGET_DIR=../$dir/operators/cass-operator-community/$VERSION
    mkdir $TARGET_DIR
    cp -R bundle/* $TARGET_DIR

    cd $TARGET_DIR
    git checkout -b cass-operator-$VERSION main
    git add .
    git commit -s -am "operator cass-operator-community (${VERSION})"
    cd -
done
