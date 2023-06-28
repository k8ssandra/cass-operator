#!/bin/sh

cat << EOF >> config/rbac/kustomization.yaml
# Add Openshift nonroot Role
- nonroot_role.yaml
EOF
