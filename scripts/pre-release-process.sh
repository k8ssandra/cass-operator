#!/bin/sh

TAG=$1

# Modify CHANGELOG automatically to match the tag
sed -i -e "s/## unreleased/## $TAG/" CHANGELOG.md

# Modify README to include proper installation refs

# Modify kustomize tags for server-system-logger and cass-operator
