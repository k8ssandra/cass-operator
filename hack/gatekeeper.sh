#!/usr/bin/env bash

helm repo add gatekeeper https://open-policy-agent.github.io/gatekeeper/charts
helm install gatekeeper/gatekeeper --name-template=gatekeeper --namespace gatekeeper-system --create-namespace --set replicas=1

# Apply ConstraintTemplates
kustomize build hack/gatekeeper | kubectl apply -f -

# Wait for CRDs to appear
CRDS=(
  k8spspreadonlyrootfilesystem.constraints.gatekeeper.sh
  k8spspcapabilities.constraints.gatekeeper.sh
  k8spspallowprivilegeescalationcontainer.constraints.gatekeeper.sh
  k8spspprivilegedcontainer.constraints.gatekeeper.sh
  k8spspallowedusers.constraints.gatekeeper.sh
)

for crd in "${CRDS[@]}"; do
  kubectl wait --for=create --timeout=60s crd/$crd
done

# Apply Constraints
kubectl apply -f - <<EOF
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sPSPReadOnlyRootFilesystem
metadata:
  name: psp-readonlyrootfilesystem
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
    excludedNamespaces:
      - kube-system
      - cert-manager
      - local-path-storage
---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sPSPCapabilities
metadata:
  name: capabilities
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
    excludedNamespaces:
      - kube-system
      - cert-manager
      - local-path-storage
---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sPSPAllowPrivilegeEscalationContainer
metadata:
  name: psp-allow-privilege-escalation-container
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
    excludedNamespaces:
      - kube-system
      - cert-manager
      - local-path-storage
---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sPSPPrivilegedContainer
metadata:
  name: psp-privileged-container
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
    excludedNamespaces:
      - kube-system
      - cert-manager
      - local-path-storage
---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sPSPAllowedUsers
metadata:
  name: psp-pods-allowed-non-root
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
  parameters:
    runAsUser:
      rule: MustRunAsNonRoot
EOF