#!/bin/sh

cat << EOF >> config/rbac/kustomization.yaml
# Add Openshift nonroot Role and ServiceAccount
- nonroot_role.yaml
- service_account_nonroot.yaml
EOF

yq eval -i '.olmDeployment = true' config/components/webhook/controller_manager_config.yaml
