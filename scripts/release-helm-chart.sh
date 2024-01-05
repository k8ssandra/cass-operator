#!/bin/sh

if [[ ! $0 == scripts/* ]]; then
    echo "This script must be run from the root directory"
    exit 1
fi

# This script assumes k8ssandra is checked out at ../k8ssandra and is checked out at main
if [ "$#" -ne 1 ]; then
    echo "Usage: scripts/release-helm-chart.sh version"
    echo "Script assumes you are in the correct branch / tag and that k8ssandra repository"
    echo "has been checked out to ../k8ssandra/"
    exit
fi

# This should work with BSD/MacOS mktemp and GNU one
CRD_TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t 'crd')

VERSION=$1
CHART_HOME=../k8ssandra/charts/cass-operator
CRD_TARGET_PATH=$CRD_TMP_DIR
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

# Add Helm conditionals to the end and beginnin of CRDs before applying them to the templates path
echo "Updating CRDs in" $TEMPLATE_HOME
CRD_FILE_NAME=$TEMPLATE_HOME/crds.yaml
echo '{{- if .Values.updateCRDs }}' > $CRD_FILE_NAME

declare -a files
files=($CRD_TARGET_PATH/*)
for i in ${!files[@]}; do
    echo "Processing " ${files[$i]}
    cat ${files[$i]} >> $CRD_FILE_NAME
    if [[ $i -lt ${#files[@]}-1 ]]; then
        echo "---" >> $CRD_FILE_NAME
    fi
done
echo '{{- end }}' >> $CRD_FILE_NAME

rm -fr $CRD_TMP_DIR

# Update version of the Chart.yaml automatically (to next minor one)
CURRENT_VERSION=$(yq '.version' $CHART_HOME/Chart.yaml)
. scripts/lib.sh
next_minor_version
echo "Updating Chart.yaml version to next minor version" $NEXT_VERSION
yq -i '.version = "'"$NEXT_VERSION"'"' $CHART_HOME/Chart.yaml

# Update appVersion to the same as version
echo "Setting appVersion to" $VERSION
yq -i '.appVersion = "'"$VERSION"'"' $CHART_HOME/Chart.yaml

# Update imageConfig settings to the current one
echo "Updating .Values.imageConfig to match config/manager/image_config.yaml"
SYSTEM_LOGGER_IMAGE=$(yq '.images.system-logger' config/manager/image_config.yaml)
K8SSANDRA_CLIENT_IMAGE=$(yq '.images.k8ssandra-client' config/manager/image_config.yaml)
CONFIG_BUILDER_IMAGE=$(yq '.images.config-builder' config/manager/image_config.yaml)

yq -i '.imageConfig.systemLogger = "cr.k8ssandra.io" + "/" + "'"$SYSTEM_LOGGER_IMAGE"'"' $CHART_HOME/values.yaml
yq -i '.imageConfig.k8ssandraClient = "cr.k8ssandra.io" + "/" + "'"$K8SSANDRA_CLIENT_IMAGE"'"' $CHART_HOME/values.yaml
yq -i '.imageConfig.configBuilder = "cr.dtsx.io" + "/" + "'"$CONFIG_BUILDER_IMAGE"'"' $CHART_HOME/values.yaml