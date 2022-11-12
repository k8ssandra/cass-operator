#!/bin/bash

RE='[^0-9]*\([0-9]*\)[.]\([0-9]*\)[.]\([0-9]*\)\([0-9A-Za-z-]*\)'

CURRENT_VERSION=$(gawk 'match($0, /^VERSION \?= /) { print substr($0, RLENGTH+1)}' Makefile)
MINOR=$(echo $CURRENT_VERSION | sed -e "s#$RE#\2#")

NEXT_VERSION_PARTS=()
for i in {1..3}
do
    PART=$(echo $CURRENT_VERSION | sed -e "s#$RE#\\$i#")
    if [ $i == 2 ]
    then
        let PART+=1
    fi
    NEXT_VERSION_PARTS+=($PART)
done

NEXT_VERSION=${NEXT_VERSION_PARTS[0]}.${NEXT_VERSION_PARTS[1]}.${NEXT_VERSION_PARTS[2]}

sed -i -e "s/VERSION ?= $CURRENT_VERSION/VERSION ?= $NEXT_VERSION/" Makefile
