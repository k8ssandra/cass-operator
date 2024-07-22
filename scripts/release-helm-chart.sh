#!/bin/sh

if [[ ! $0 == scripts/* ]]; then
    echo "This script must be run from the root directory"
    exit 1
fi

# This script assumes k8ssandra is checked out at ../k8ssandra and is checked out at main
if [ "$#" -le 1 ]; then
    echo "Usage: scripts/release-helm-chart.sh version legacy"
    echo "Script assumes you are in the correct branch / tag and that k8ssandra repository"
    echo "has been checked out to ../k8ssandra/. If legacy is set, the script will generate"
    echo "CRDs to the chart/crds directory"
    exit
fi

# Includes here to get all the updates even if we swap to an older branch
. scripts/lib.sh

LEGACY=false
if [[ $2 == "legacy" ]]; then
    LEGACY=true
fi

# This should work with BSD/MacOS mktemp and GNU one
CRD_TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t 'crd')

VERSION=$1
CHART_HOME=../k8ssandra/charts/cass-operator
TEMPLATE_HOME=$CHART_HOME/templates
CRD_TARGET_PATH=$TEMPLATE_HOME

# Checkout tag
git checkout v$VERSION

# Create CRDs
kustomize build config/crd  --output $CRD_TMP_DIR

# Rename generated CRDs to shorter format
for f in $CRD_TMP_DIR/*; do
    TARGET_FILENAME=$(yq '.spec.names.plural' $f).yaml
    mv $f $CRD_TMP_DIR/$TARGET_FILENAME
done

if [ "$LEGACY" == true ]; then
    echo "Updating CRDs for legacy CRD handling in Helm chart"

    # Update CRDs for legacy Helm chart
    CRD_TARGET_PATH=$CHART_HOME/crds
    cp -r $CRD_TMP_DIR/* $CRD_TARGET_PATH
else
    # Add Helm conditionals to the end and beginning of CRDs before applying them to the templates path
    echo "Updating CRDs in" $TEMPLATE_HOME
    CRD_FILE_NAME=$CRD_TARGET_PATH/crds.yaml
    echo '{{- if .Values.manageCrds }}' > $CRD_FILE_NAME

    declare -a files
    files=($CRD_TMP_DIR/*)
    for i in ${!files[@]}; do
        echo "Processing " ${files[$i]}
        yq -i '.metadata.annotations."helm.sh/resource-policy" = "keep"' ${files[$i]}
        cat ${files[$i]} >> $CRD_FILE_NAME
        if [[ $i -lt ${#files[@]}-1 ]]; then
            echo "---" >> $CRD_FILE_NAME
        fi
    done
    echo '{{- end }}' >> $CRD_FILE_NAME
fi

rm -fr $CRD_TMP_DIR

# Update role.yaml
echo "Updating role.yaml"
ROLE_FILE_NAME=$TEMPLATE_HOME/role.yaml
cat <<'EOF' > $ROLE_FILE_NAME
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "k8ssandra-common.fullname" . }}
  labels: {{ include "k8ssandra-common.labels" . | indent 4 }}
{{- if .Values.global.clusterScoped }}
EOF
yq -N eval-all '.rules = (.rules as $item ireduce ([]; . *+ $item)) | select(di == 0) | with_entries(select(.key | test("rules")))' config/rbac/role.yaml >> $ROLE_FILE_NAME
echo '{{- else }}' >> $ROLE_FILE_NAME
yq -N 'select(di == 0) | with_entries(select(.key | test("rules")))' config/rbac/role.yaml >> $ROLE_FILE_NAME
cat <<'EOF' >> $ROLE_FILE_NAME
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ template "k8ssandra-common.fullname" . }}
  labels: {{ include "k8ssandra-common.labels" . | indent 4 }}
EOF
yq -N 'select(di == 1) | with_entries(select(.key | test("rules")))' config/rbac/role.yaml >> $ROLE_FILE_NAME
echo '{{- end }}' >> $ROLE_FILE_NAME

# Update version of the Chart.yaml automatically (to next minor one)
CURRENT_VERSION=$(yq '.version' $CHART_HOME/Chart.yaml)
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

yq -i '.imageConfig.systemLogger = "'"$SYSTEM_LOGGER_IMAGE"'"' $CHART_HOME/values.yaml
yq -i '.imageConfig.k8ssandraClient = "'"$K8SSANDRA_CLIENT_IMAGE"'"' $CHART_HOME/values.yaml
yq -i '.imageConfig.configBuilder = "'"$CONFIG_BUILDER_IMAGE"'"' $CHART_HOME/values.yaml