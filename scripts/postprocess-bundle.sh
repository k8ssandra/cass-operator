#!/bin/sh

# These labels are required to be in the bundle.Dockerfile, but can't be added by the operator-sdk automatically
cat <<EOF >> bundle.Dockerfile
# Certified Openshift required labels
LABEL com.redhat.openshift.versions="v4.5"
LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.delivery.backport=true
EOF

# Add them to the bundle metadata also
yq eval -i '.annotations."com.redhat.openshift.versions" = "v4.5"' bundle/metadata/annotations.yaml
yq eval -i '.annotations."com.redhat.delivery.operator.bundle" = true' bundle/metadata/annotations.yaml
yq eval -i '.annotations."com.redhat.delivery.backport" = true' bundle/metadata/annotations.yaml
yq eval -i '.annotations."com.redhat.openshift.versions" headComment = "Certified Openshift required labels"' bundle/metadata/annotations.yaml

# This file is extra from creation process on config/manifests, should not be in the bundle itself
rm -f bundle/manifests/field-config_v1_configmap.yaml

# Use yq to set that date to the createdAt field
createdAt=$(date +%Y-%m-%d) yq eval '.metadata.annotations.createdAt = env(createdAt)' -i bundle/manifests/cass-operator.clusterserviceversion.yaml 

# Use the correct containerImage from the deployment
yq eval '.metadata.annotations.containerImage = .spec.install.spec.deployments[0].spec.template.spec.containers[0].image' -i bundle/manifests/cass-operator.clusterserviceversion.yaml

# Add ds.png as base64 encoded icondata
yq eval '.spec.icon[0].base64data = "'"$(base64 -w 80 -i config/manifests/images/ds.png)"'"' -i bundle/manifests/cass-operator.clusterserviceversion.yaml
