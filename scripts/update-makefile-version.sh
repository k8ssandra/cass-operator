#!/bin/bash
if [[ ! $0 == scripts/* ]]; then
    echo "This script must be run from the root directory"
    exit 1
fi

. scripts/lib.sh

CURRENT_VERSION=$(gawk 'match($0, /^VERSION \?= /) { print substr($0, RLENGTH+1)}' Makefile)

if [[ -n $PATCH_RELEASE ]]; then
    next_patch_version
else
    next_minor_version
fi

sed -i '' -e "s/VERSION ?= $CURRENT_VERSION/VERSION ?= $NEXT_VERSION/" Makefile
