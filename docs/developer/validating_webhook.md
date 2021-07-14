## The validating webhook.

The operator offers, and installs when possible, a validating webhook for
related CRDs. The webhook is intended to provide checks of the validity of an
update or create request, where there might be CRD-specific guardrails that are
not readily checked by implicit CRD validity. Such checks include preventing
renaming certain elements of the deployment, such as the the cassandra cluster
or the racks, which are core to the identity of a cassandra cluster.

Validating webhooks have specific requirements in kubernetes:
* They must be served over TLS
* The TLS service name where they are reached must match the subject of the certificate
* The CA signing the certificate must be either installed in the kube apiserver filesystem, or
explicitly configured in the kubernetes validatingwebhookconfiguration object.

To support the above scenarios, the operator uses cert-manager to take care of generating and
injecting the certificates. The validating webhook can be disabled in the OperatorConfig, which is
mounted as a ConfigMap to the container.