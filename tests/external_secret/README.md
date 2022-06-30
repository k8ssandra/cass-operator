## Install Vault

helm install vault hashicorp/vault --set "server.dev.enabled=true" --namespace cass-operator
# --set "csi.enabled=true"

## Go into Vault and exec certain commands..

kubectl exec -it vault-0 -- /bin/sh

vault secrets enable -path=internal kv-v2

vault kv put internal/database/config username="db-readonly-username" password="db-secret-password"

vault auth enable kubernetes

vault write auth/kubernetes/config \
    kubernetes_host="https://$KUBERNETES_PORT_443_TCP_ADDR:443"

vault policy write internal-app - <<EOF
path "internal/data/database/config" {
  capabilities = ["read"]
}
EOF

vault write auth/kubernetes/role/internal-app \
    bound_service_account_names=cass-operator-controller-manager \
    bound_service_account_namespaces=cass-operator \
    policies=internal-app \
    ttl=24h

## Now create the DC:

kubectl apply -f tests/testdata/default-single-rack-single-node-dc-vault.yaml
