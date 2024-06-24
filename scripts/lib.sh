#!/bin/sh

NEXT_VERSION=""
VERSION_PART=""

next_minor_version() {
    VERSION_PART=2
    next_version
}

next_patch_version() {
    VERSION_PART=3
    next_version
}

next_version() {
    local RE='[^0-9]*\([0-9]*\)[.]\([0-9]*\)[.]\([0-9]*\)\([0-9A-Za-z-]*\)'

    local NEXT_VERSION_PARTS=(0 0 0)
    for i in {1..3}
    do
        local PART=$(echo $CURRENT_VERSION | sed -e "s#$RE#\\$i#")
        ARRAY_INDEX=$((i-1))
        NEXT_VERSION_PARTS[$ARRAY_INDEX]=$PART

        if [ $i == $VERSION_PART ]
        then
            let PART+=1
            NEXT_VERSION_PARTS[$ARRAY_INDEX]=$PART
            break
        fi
    done

    NEXT_VERSION=${NEXT_VERSION_PARTS[0]}.${NEXT_VERSION_PARTS[1]}.${NEXT_VERSION_PARTS[2]}
}
