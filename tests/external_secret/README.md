## Install Helm repositories

```
helm repo add secrets-store-csi-driver https://kubernetes-sigs.github.io/secrets-store-csi-driver/charts
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update
```

## Install Vault

```
helm install vault hashicorp/vault --set "server.dev.enabled=true" --namespace cass-operator
# --set "csi.enabled=true"
```

## Go into Vault and exec certain commands..

```
kubectl exec -it vault-0 -- /bin/sh

vault secrets enable -path=internal kv-v2

vault kv put internal/database/config superuser="superpassword"

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
```

## Install CSI driver:

Not sure if syncSecret is needed, but Vault documentation wants it..

```
helm install csi secrets-store-csi-driver/secrets-store-csi-driver \
    --set syncSecret.enabled=true --namespace cass-operator
```

Create the SecretProviderClass:

```yaml
apiVersion: secrets-store.csi.x-k8s.io/v1
kind: SecretProviderClass
metadata:
  name: vault-database
spec:
  provider: vault
  parameters:
    vaultAddress: "http://vault.default:8200"
    roleName: "internal-app"
    objects: |
      - objectName: "superuser"
        secretPath: "internal/database/config"
        secretKey: "superuser"
```

The objectName becomes the username and the secretKey's data becomes the password.

## Now create the DC:

```
kubectl apply -f tests/testdata/default-single-rack-single-node-dc-vault.yaml
```
