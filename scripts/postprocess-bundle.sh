#!/bin/sh

# These labels are required to be in the bundle.Dockerfile, but can't be added by the operator-sdk automatically
cat <<EOF >> bundle.Dockerfile
# Certified Openshift required labels
LABEL com.redhat.openshift.versions="v4.9"
LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.delivery.backport=true
EOF

# Add them to the bundle metadata also
yq eval -i '.annotations."com.redhat.openshift.versions" = "v4.9"' bundle/metadata/annotations.yaml
yq eval -i '.annotations."com.redhat.delivery.operator.bundle" = true' bundle/metadata/annotations.yaml
yq eval -i '.annotations."com.redhat.delivery.backport" = true' bundle/metadata/annotations.yaml
yq eval -i '.annotations."com.redhat.openshift.versions" headComment = "Certified Openshift required labels"' bundle/metadata/annotations.yaml

# This file is extra from creation process on config/manifests, should not be in the bundle itself
rm -f bundle/manifests/field-config_v1_configmap.yaml

# Use yq to set that date to the createdAt field
createdAt=$(date +%Y-%m-%d) yq eval '.metadata.annotations.createdAt = env(createdAt)' -i bundle/manifests/cass-operator.clusterserviceversion.yaml 

REGISTRY=$1
if [ -n "${REGISTRY}" ]; then
    # Modify image to have repository prefix (docker.io/k8ssandra/cass-operator)
    yq eval '.spec.install.spec.deployments[0].spec.template.spec.containers[0].image |= "'"$REGISTRY"'" + "/" + .' -i bundle/manifests/cass-operator.clusterserviceversion.yaml
fi

# Use the correct containerImage from the deployment
yq eval '.metadata.annotations.containerImage = .spec.install.spec.deployments[0].spec.template.spec.containers[0].image' -i bundle/manifests/cass-operator.clusterserviceversion.yaml

# base64 command differs in every OS it seems..
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    BASE64_DS_IMG=$(base64 -w 80 -i config/manifests/images/ds.png)
elif [[ "$OSTYPE" == "darwin"* ]]; then
    BASE64_DS_IMG=$(base64 -b 80 -i config/manifests/images/ds.png)
elif [[ "$OSTYPE" == "freebsd"* ]]; then
    # FreeBSD seems to by default set 72 chars, which is fine
    BASE64_DS_IMG=$(base64 -e config/manifests/images/ds.png)
else
    # No idea what system this could be.. lets assume gnu tools
    BASE64_DS_IMG=$(base64 -w 80 -i config/manifests/images/ds.png)
fi

# Add ds.png as base64 encoded icondata
yq eval '.spec.icon[0].base64data = "'"$BASE64_DS_IMG"'"' -i bundle/manifests/cass-operator.clusterserviceversion.yaml
