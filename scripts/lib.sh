#!/bin/sh

NEXT_VERSION=""

next_minor_version() {
    local RE='[^0-9]*\([0-9]*\)[.]\([0-9]*\)[.]\([0-9]*\)\([0-9A-Za-z-]*\)'

    local MINOR=$(echo $CURRENT_VERSION | sed -e "s#$RE#\2#")

    local NEXT_VERSION_PARTS=()
    for i in {1..3}
    do
        local PART=$(echo $CURRENT_VERSION | sed -e "s#$RE#\\$i#")
        if [ $i == 2 ]
        then
            let PART+=1
        fi
        NEXT_VERSION_PARTS+=($PART)
    done

    NEXT_VERSION=${NEXT_VERSION_PARTS[0]}.${NEXT_VERSION_PARTS[1]}.0
}
