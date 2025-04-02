#!/usr/bin/env bash
if [[ ! $0 == scripts/* ]]; then
    echo "This script must be run from the root directory"
    exit 1
fi

if [ "$#" -le 1 ]; then
    echo "Usage: scripts/update-dse-ci.sh version"
    exit 1
fi

DSE_VERSION=$1

# Update kindIntegTest.yaml with yq to use the new DSE version and a newer image tag
# We can use the dse-server repository for the test as it includes enough modern mgmt-api now
