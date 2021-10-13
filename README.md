# Cass Operator
[![License: Apache License 2.0](https://img.shields.io/github/license/k8ssandra/cass-operator)](https://github.com/k8ssandra/cass-operator/blob/master/LICENSE.txt)

The DataStax Kubernetes Operator for Apache Cassandra&reg;. This repository replaces the old [datastax/cass-operator](https://github.com/datastax/cass-operator) for use-cases in the k8ssandra project. Some documentation is still out of date and will be modified in the future. Check [k8ssandra/k8ssandra](https://github.com/k8ssandra/k8ssandra) for more up to date information.

## Getting Started

To create a full featured cluster, the recommend approach is to use the Helm charts from k8ssandra. Check the [Getting started](https://k8ssandra.io/docs/getting-started/) documentation at (k8ssandra.io)[https://k8ssandra.io/docs].

A more custom approach is to use Kustomize as described in the following sections if you wish to use cass-operator only. If updating from previous version, please see ``Upgrade instructions`` section first.

### Installing the operator with Kustomize

The operator can be installed in a namespace scoped settings or cluster wide. If installed cluster wide, one can define which namespaces (or all) are watched for ``CassandraDatacenter`` objects.

Default installation is simple, the kubectl will create a namespace ``cass-operator`` and install cass-operator there. It will only listen for the CassandraDatacenters in that namespace. Note that since the manifests will install a [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), the user running the commands will need cluster-admin privileges.

Default install requires cert-manager to be installed, since webhooks require TLS certificates to be injected. See below how to install cert-manager if your environment does not have it installed previously.

```console
kubectl apply -k github.com/k8ssandra/cass-operator/config/deployments/default
```

If you wish to install it with cluster wide rights to monitor all the namespaces for ``CassandraDatacenter`` objects, use the following command:

```console
kubectl apply -k github.com/k8ssandra/cass-operator/config/deployments/cluster
```

Alternatively, if you checkout the code, you can use ``make deploy`` to run [Kustomize](https://kustomize.io/) and deploy the files.

This will deploy the operator, along with any requisite resources such as Role, RoleBinding, etc., to the `cass-operator` namespace. You can check to see if the operator is ready as follows:

```console
$ kubectl -n cass-operator get pods --selector name=cass-operator
NAME                             READY   STATUS    RESTARTS   AGE
cass-operator-555577b9f8-zgx6j   1/1     Running   0          25h
```

### Upgrade instructions:

Updates are supported from previous versions of ``k8ssandra/cass-operator``. If upgrading from versions older than 1.7.0 (released under ``datastax/cass-operator`` name), please upgrade first to version 1.7.1. The following instructions apply when upgrading from 1.7.1 to 1.8.0.

Due to the modifications to cass-operator’s underlying controller-runtime and updated Kubernetes versions, there is a need to do couple of manual steps before updating to a newest version of cass-operator. Newer Kubernetes versions require stricter validation and as such we need to remove ``preserveUnknownFields`` global property from the CRD to allow us to update to a newer CRD. The newer controller-runtime on the other hand modifies the liveness, readiness and configuration options, which require us to delete the older deployment. These commands do not delete your running Cassandra instances.

 Run the following commands assuming cass-operator is installed to ``cass-operator`` namespace (change -n parameter if it is installed to some other namespace):

```sh
kubectl -n cass-operator delete deployment.apps/cass-operator
kubectl -n cass-operator delete service/cassandradatacenter-webhook-service
kubectl patch crd cassandradatacenters.cassandra.datastax.com -p '{"spec":{"preserveUnknownFields":false}}'
```

You can now install new version of cass-operator as instructed previously.

### Install Prometheus monitor rules

If you have Prometheus installed in your cluster, you can apply the following command to install the Prometheus support:

```console
kubectl apply -k github.com/k8ssandra/cass-operator/config/prometheus
```

### Install cert-manager

We have tested the cass-operator to work with cert-manager versions 1.3.1 and 1.5.3. Other versions might work also. To install 1.5.3 to your cluster, run the following command:

```console
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.5.3/cert-manager.yaml
```

#### Modifying the Kustomize template

If you wish to modify the deployment, create your own ``kustomization.yaml`` and modify it to your needs. The starting point could be the ``deployments/config/default`` and we'll add a cluster scoped installation as our component:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: cass-operator

resources:
  - github.com/k8ssandra/cass-operator/config/deployments/default?ref=v1.8.0

components:
  - github.com/k8ssandra/cass-operator/config/components/cluster

images:
- name: k8ssandra/cass-operator
  newTag: v1.8.0
```

We provide both components to modify the installation as well as some additional resources for custom features. At the moment, you can modify the behavior of the installation in the following ways, or remove a component to
ignore the feature (components enabled in the default installation are marked with asterisk). Apply ``github.com/k8ssandra/cass-operator/config/components/`` before component name if using remote installation:

| Component name | Description |
| ------------- | ------------- |
| namespace | Create namespace before installation* |
| webhook | Enable validation webhooks in cass-operator (requires cert-manager) * |
| clusterscope | Install cass-operator in a cluster scope, monitoring all the namespaces |
| auth-proxy | Protect Prometheus /metrics endpoint with api-server authentication |

And following resource. Apply ``github.com/k8ssandra/cass-operator/config/`` before resource name if using remote installation:

| Resource | Description |
| ------------- | ------------- |
| prometheus | Add metrics scraping for Prometheus |

You can find more resources on how Kustomize works from their [documentation](https://kubectl.docs.kubernetes.io/installation/kustomize/). You can install kustomize with ``make kustomize`` if you do not have it already (this will install it to ``bin/kustomize``).

### Creating a storage class

If the ``default`` StorageClass is not suitable for use (volumeBindingMode WaitForFirstConsumer is required) or you wish to use different one, you will need to create an appropriate storage class which will define the type of storage to use for Cassandra nodes in a cluster. For example, here is a storage class for using SSDs in GKE:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: server-storage
provisioner: kubernetes.io/gce-pd
parameters:
  type: pd-ssd
  replication-type: none
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
```

Paste the above to a file and apply:

```
kubectl apply -f https://raw.githubusercontent.com/k8ssandra/cass-operator/v1.8.0/operator/k8s-flavors/gke/storage.yaml
```

### Creating a CassandraDatacenter

The following resource defines a Cassandra 3.11.11 datacenter with 3 nodes on one rack, which you can also find at [config/samples/cassandra-3.11.x/example-cassdc-minimal.yaml](config/samples/cassandra-3.11.x/example-cassdc-minimal.yaml):

```yaml
apiVersion: cassandra.datastax.com/v1beta1
kind: CassandraDatacenter
metadata:
  name: dc1
spec:
  clusterName: cluster1
  serverType: cassandra
  serverVersion: 3.11.11
  managementApiAuth:
    insecure: {}
  size: 3
  storageConfig:
    cassandraDataVolumeClaimSpec:
      storageClassName: server-storage
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 5Gi
  config:
    cassandra-yaml:
      authenticator: org.apache.cassandra.auth.PasswordAuthenticator
      authorizer: org.apache.cassandra.auth.CassandraAuthorizer
      role_manager: org.apache.cassandra.auth.CassandraRoleManager
    jvm-options:
      initial_heap_size: 800M
      max_heap_size: 800M
```

Apply the above as follows:

```console
kubectl -n cass-operator apply -f https://raw.githubusercontent.com/k8ssandra/cass-operator/master/operator/example-cassdc-yaml/cassandra-3.11.x/example-cassdc-minimal.yaml
```

You can check the status of pods in the Cassandra cluster as follows:

```console
$ kubectl -n cass-operator get pods --selector cassandra.datastax.com/cluster=cluster1
NAME                         READY   STATUS    RESTARTS   AGE
cluster1-dc1-default-sts-0   2/2     Running   0          26h
cluster1-dc1-default-sts-1   2/2     Running   0          26h
cluster1-dc1-default-sts-2   2/2     Running   0          26h
```

You can check to see the current progress of bringing the Cassandra datacenter online by checking the `cassandraOperatorProgress` field of the `CassandraDatacenter`'s `status` sub-resource as follows:

```console
$ kubectl -n cass-operator get cassdc/dc1 -o "jsonpath={.status.cassandraOperatorProgress}"
Ready
```

(`cassdc` and `cassdcs` are supported short forms of `CassandraDatacenter`.)

A value of "Ready", as above, means the operator has finished setting up the Cassandra datacenter.

You can also check the Cassandra cluster status using `nodetool` by invoking it on one of the pods in the Cluster as follows:

```console
$ kubectl -n cass-operator exec -it -c cassandra cluster1-dc1-default-sts-0 -- nodetool status
Datacenter: dc1
===============
Status=Up/Down
|/ State=Normal/Leaving/Joining/Moving/Stopped
--  Address         Load       Tokens       Owns (effective)  Host ID                               Rack
UN  10.233.105.125  224.82 KiB  1            65.4%             5e29b4c9-aa69-4d53-97f9-a3e26115e625  r1
UN  10.233.92.96    186.48 KiB  1            61.6%             b119eae5-2ff4-4b06-b20b-c492474e59a6  r1
UN  10.233.90.54    205.1 KiB   1            73.1%             0a96e814-dcf6-48b9-a2ca-663686c8a495  r1
```

The operator creates a secure Cassandra cluster by default, with a new superuser (not the traditional `cassandra` user) and a random password. You can get those out of a Kubernetes secret and use them to log into your Cassandra cluster for the first time. For example:

```console
$ # get CASS_USER and CASS_PASS variables into the current shell
$ CASS_USER=$(kubectl -n cass-operator get secret cluster1-superuser -o json | jq -r '.data.username' | base64 --decode)
$ CASS_PASS=$(kubectl -n cass-operator get secret cluster1-superuser -o json | jq -r '.data.password' | base64 --decode)
$ kubectl -n cass-operator exec -ti cluster1-dc1-default-sts-0 -c cassandra -- sh -c "cqlsh -u '$CASS_USER' -p '$CASS_PASS'"

Connected to cluster1 at 127.0.0.1:9042.
[cqlsh 5.0.1 | Cassandra 3.11.6 | CQL spec 3.4.4 | Native protocol v4]
Use HELP for help.

cluster1-superuser@cqlsh> select * from system.peers;

 peer      | data_center | host_id                              | preferred_ip | rack    | release_version | rpc_address | schema_version                       | tokens
-----------+-------------+--------------------------------------+--------------+---------+-----------------+-------------+--------------------------------------+--------------------------
 10.28.0.4 |         dc1 | 4bf5e110-6c19-440e-9d97-c013948f007c |         null | default |          3.11.6 |   10.28.0.4 | e84b6a60-24cf-30ca-9b58-452d92911703 | {'-7957039572378599263'}
 10.28.5.5 |         dc1 | 3e84b0f1-9c1e-4deb-b6f8-043731eaead4 |         null | default |          3.11.6 |   10.28.5.5 | e84b6a60-24cf-30ca-9b58-452d92911703 | {'-3984092431318102676'}

(2 rows)
```

### Installing cluster via Helm

To install a cluster with optional integrated backup/restore and repair utilities, check the [k8ssandra/k8ssandra](https://github.com/k8ssandra/k8ssandra) helm charts project. 

If you wish to install only the cass-operator, you can run the following command:

```
helm repo add k8ssandra https://helm.k8ssandra.io/stable
helm install cass-operator k8ssandra/cass-operator -n cass-operator --create-namespace
```

## Features

- Proper token ring initialization, with only one node bootstrapping at a time
- Seed node management - one per rack, or three per datacenter, whichever is more
- Server configuration integrated into the CassandraDatacenter CRD
- Rolling reboot nodes by changing the CRD
- Store data in a rack-safe way - one replica per cloud AZ
- Scale up racks evenly with new nodes
- Scale down racks evenly by decommissioning existing nodes
- Replace dead/unrecoverable nodes
- Multi DC clusters (limited to one Kubernetes namespace)

All features are documented in the [User Documentation](docs/user/README.md).

### Containers

The operator is comprised of the following container images working in concert:
* The operator, built from sources using the kubebuilder v3 structure, from [controllers](controllers/) directory.
* The config builder init container, built from sources in [datastax/cass-config-builder](https://github.com/datastax/cass-config-builder).
* server-system-logger, a tiny tail logger for outputting Cassandra logs for kubectl. Implemented in [logger.Dockerfile](logger.Dockerfile)
* Cassandra, built from
  [datastax/management-api-for-apache-cassandra](https://github.com/k8ssandra/management-api-for-apache-cassandra),
  with Cassandra 3.11.11 and 4.0.1 support
* ... or DSE, built from [datastax/docker-images](https://github.com/datastax/docker-images).

The Cassandra container must be built with support for management-api, otherwise cass-operator will fail to work correctly.

### Overriding properties of cass-operator created Containers

If the CassandraDatacenter specifies a podTemplateSpec field, then containers with specific names can be used to override default settings in containers that will be created by cass-operator.

Currently cass-operator will create an init container with the name of "server-config-init". Containers that will be created have the names "cassandra" and "server-system-logger".

In general, the values specified in this way by the user will override anything generated by cass-operator.

Of special note is that user-specified environment variables, ports, and volumes in the corresponding containers will be added to the values that cass-operator automatically generates for those containers.

```yaml
apiVersion: cassandra.datastax.com/v1beta1
kind: CassandraDatacenter
metadata:
  name: dc1
spec:
  clusterName: cluster1
  serverType: cassandra
  serverVersion: 3.11.11
  managementApiAuth:
    insecure: {}
  size: 3
  podTemplateSpec:
    spec:
      initContainers:
        - name: "server-config-init"
          env:
          - name: "EXTRA_PARAM"
            value: "123"
      containers:
        - name: "cassandra"
          terminationMessagePath: "/dev/other-termination-log"
          terminationMessagePolicy: "File"
  storageConfig:
    cassandraDataVolumeClaimSpec:
      storageClassName: server-storage
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 5Gi
  config:
    cassandra-yaml:
      authenticator: org.apache.cassandra.auth.PasswordAuthenticator
      authorizer: org.apache.cassandra.auth.CassandraAuthorizer
      role_manager: org.apache.cassandra.auth.CassandraRoleManager
    jvm-options:
      initial_heap_size: 800M
      max_heap_size: 800M
```

## Requirements

- Kubernetes cluster, 1.17 or newer.

## Contributing

If you wish to file a bug, enhancement proposal or have other questions, use the issues in this repository or in the [k8ssandra/k8ssandra](https://github.com/k8ssandra/k8ssandra) repository. PRs should target this repository and you can link the PR to issue repository with ``k8ssandra/k8ssandra#ticketNumber`` syntax.

For other means of contacting, check [k8ssandra community](https://k8ssandra.io/community/) resources.

### Developer setup

Almost every build, test, or development task requires the following
pre-requisites...

* Golang 1.15 or newer
* Docker, either the docker.io packages on Ubuntu, Docker Desktop for Mac,
  or your preferred docker distribution. Other container engines such as podman should work also.

### Building

The operator uses Makefiles for its build process. 

#### Build the Operator Container Image
This build task will create the operator container image, building or rebuilding the binary from golang sources if necessary:

``` bash
make docker-build
```

#### Build the Operator Binary
If you wish to perform ONLY to the golang build or rebuild, without creating
a container image:

``` bash
make build
```

Or just ``make``.

#### Build and deploy to kind
To simplify testing processes, the following command will build the docker image and load it to the kind instance.

```bash
make docker-kind
```

### Testing

Tests are separated to unit-tests (including [envtests](https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html)) and end-to-end tests. To run the unit tests and envtests, use:

``` bash
make test
```

test target will spawn a envtest environment, which will require the ports to be available.  

#### End-to-end Automated Testing

Run fully automated end-to-end tests (these will take a while and require a Kubernetes cluster access with up to 6 worker nodes):

```bash
make integ-test
```

To run a single e2e test:

```bash
M_INTEG_DIR=test_dir make integ-test
```

More details of end-to-end testing are [here](tests/README.md).

## Uninstall

*This will destroy all of your data!*

Delete your CassandraDatacenters first, otherwise Kubernetes will block deletion of the namespace because we use a finalizer. The following command deletes all CassandraDatacenters from all namespaces:

```
kubectl delete cassdcs --all-namespaces --all
```

If you used the ``make deploy`` to deploy the operator, replace it with ``make undeploy`` to uninstall.

## Contacts

For questions, please reach out on [k8ssandra Community](https://k8ssandra.io/community/) channels. For development questions, we are active on our Discord channel #k8ssandra-dev. Or you can open an issue. 

## License

Copyright DataStax, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.

## Dependencies

For information on the packaged dependencies of Cass Operator and their licenses, check out our [open source report](https://app.fossa.com/reports/ed8a8cc0-4bb4-405b-b07c-5316f9b524f5).
