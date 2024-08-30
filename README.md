# Cass Operator
[![License: Apache License 2.0](https://img.shields.io/github/license/k8ssandra/cass-operator)](https://github.com/k8ssandra/cass-operator/blob/master/LICENSE.txt)

The DataStax Kubernetes Operator for Apache Cassandra&reg;. This repository replaces the old [datastax/cass-operator](https://github.com/datastax/cass-operator) for use-cases in the k8ssandra project. Some documentation is still out of date and will be modified in the future. Check [k8ssandra/k8ssandra](https://github.com/k8ssandra/k8ssandra) for more up to date information.

## Getting Started

``cass-operator`` can be used as standalone product to manage your Cassandra cluster or as part of the (https://docs.k8ssandra.io/install/)[k8ssandra-operator]. With tooling such as managed repairs and automated backups, see k8ssandra-operator as the recommended approach. 

If updating from previous versions, please see ``Upgrade instructions`` section first.

### Installing the operator with Helm

cass-operator is available as a Helm chart as part of the k8ssandra project. First add the k8ssandra Helm repository:

```console
helm repo add k8ssandra https://helm.k8ssandra.io/stable
helm repo update
```

Then to install the cass-operator using the default settings to namespace ``cass-operator``, use the following command:

```console
helm install cass-operator k8ssandra/cass-operator -n cass-operator --create-namespace
```

You can modify the installation using the values from our (values.yaml)[https://github.com/k8ssandra/k8ssandra/blob/main/charts/cass-operator/values.yaml] file.

By default, the Helm installation requires ``cert-manager`` to be present in the Kubernetes installation. If you do not have cert-manager installed, follow the steps at (https://cert-manager.io/docs/installation/helm/)[cert-manager's] documentation. If you do not wish to use cert-manager, either disable the webhooks using ``--set admissionWebhooks.enabled=false`` or provide your own certificate in a secret and set it with ``--set admissionWebhooks.customCertificate=namespace/certificate-name``. 

### Installing the operator with Kustomize

The operator can be installed in a namespace scoped settings or cluster wide. If installed cluster wide, one can define which namespaces (or all) are watched for ``CassandraDatacenter`` objects.

Default installation is simple, the kubectl will create a namespace ``cass-operator`` and install cass-operator there. It will only listen for the CassandraDatacenters in that namespace. Note that since the manifests will install a [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), the user running the commands will need cluster-admin privileges.

Default install requires cert-manager to be installed, since webhooks require TLS certificates to be injected. See below how to install cert-manager if your environment does not have it installed previously.

```console
kubectl apply --force-conflicts --server-side -k github.com/k8ssandra/cass-operator/config/deployments/default?ref=v1.22.1
```

If you wish to install it with cluster wide rights to monitor all the namespaces for ``CassandraDatacenter`` objects, use the following command:

```console
kubectl apply --force-conflicts --server-side -k github.com/k8ssandra/cass-operator/config/deployments/cluster?ref=v1.22.1
```

Alternatively, if you checkout the code, you can use ``make deploy`` to run [Kustomize](https://kustomize.io/) and deploy the files.

This will deploy the operator, along with any requisite resources such as Role, RoleBinding, etc., to the `cass-operator` namespace. You can check to see if the operator is ready as follows:

```console
$ kubectl -n cass-operator get pods --selector name=cass-operator
NAME                             READY   STATUS    RESTARTS   AGE
cass-operator-555577b9f8-zgx6j   1/1     Running   0          25h
```

#### Install Prometheus monitor rules

If you have Prometheus installed in your cluster, you can apply the following command to install the Prometheus support:

```console
kubectl apply -k github.com/k8ssandra/cass-operator/config/prometheus?ref=v1.22.1
```

#### Install cert-manager

We have tested the current cass-operator to work with cert-manager versions 1.12.2. Other versions should work also. To install 1.12.2 to your cluster, run the following command:

```console
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.2/cert-manager.yaml
```

#### Modifying the Kustomize template

If you wish to modify the deployment, create your own ``kustomization.yaml`` and modify it to your needs. The starting point could be the ``deployments/config/default`` and we'll add a cluster scoped installation as our component:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: cass-operator

resources:
  - github.com/k8ssandra/cass-operator/config/deployments/default?ref=v1.22.1

components:
  - github.com/k8ssandra/cass-operator/config/components/cluster?ref=v1.22.1
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

##### Using kustomize to modify the default registry that's used by cass-operator

cass-operator's default image name patterns and repositories are defined in a [image_config.yaml](https://github.com/k8ssandra/cass-operator/blob/master/config/manager/image_config.yaml) file. The image_config.yaml will define only the deployed Cassandra / DSE and server-system-logger images, not the cass-operator itself. To modify imageConfig when deploying, we can create a kustomize component to replace this file in the deployment with our own.

In this example, create two directories, ``our_installation`` and ``private_config_component``. You can name these as you wish, just remember to replace the directory names in the yaml files.

In ``private_config_component`` create two files, ``kustomization.yaml`` and ``image_config.yaml``. Lets say our registry is located at ``localhost:5000``, here's the ``private_config_component/image_config.yaml``:

```yaml
apiVersion: config.k8ssandra.io/v1beta1
kind: ImageConfig
metadata:
  name: image-config
images:
  system-logger: "k8ssandra/system-logger:v1.22.1"
  config-builder: "datastax/cass-config-builder:1.0-ubi7"
  imageRegistry: "localhost:5000"
defaults:
  cassandra:
    repository: "k8ssandra/cass-management-api"
  dse:
    repository: "datastax/dse-server"
    suffix: "-ubi7"
```

And for ``private_config_component/kustomization.yaml`` you will need the following:

```yaml
apiVersion: kustomize.config.k8s.io/v1alpha1
kind: Component

configMapGenerator:
- files:
  - image_config.yaml
  behavior: merge
  name: manager-config
```

Finally, the kustomization file which we'll deploy will look this (add cluster component if you wish to deploy in cluster-scope), add to ``our_installation/kustomization.yaml``:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - github.com/k8ssandra/cass-operator/config/deployments/default?ref=v1.22.1

components:
  - components/private_image_config
```

If you also wish to load the cass-operator from a different path, you will need to add the following part to the ``our_installation/kustomization.yaml``:

```yaml
images:
- name: controller
  newName: localhost:5000/k8ssandra/cass-operator
  newTag: v1.22.1
```

Run ``kubectl apply -k our_installation`` to install cass-operator.

### Installing the operator with Operator Lifecycle Manager (or installing to Openshift)

cass-operator is available in the OperatorHub as a community version as well as certified version. You can install these directly from the Openshift's UI.

For other distributions of Kubernetes, you can find cass-operator from the OperatorHub under the name (https://operatorhub.io/operator/cass-operator-community)[cass-operator-community].

If OLM is already installed in your cluster, the operator can be installed with the following command:

```
kubectl create -f https://operatorhub.io/install/cass-operator-community.yaml
```

### Upgrade instructions:

Updates are supported from previous versions of ``k8ssandra/cass-operator``. If upgrading from versions older than 1.7.0 (released under ``datastax/cass-operator`` name), please upgrade first to version 1.7.1. The following instructions apply when upgrading from 1.7.1 to 1.8.0 or newer up to 1.10.1. Upgrading to 1.11.0 if using Kubernetes 1.23 requires updating at least to 1.8.0 first, since 1.7.1 can not be used in Kubernetes 1.23 or newer.

If you're upgrading from 1.7.1, there is an additional step to take due to the modifications to cass-operator’s underlying controller-runtime and updated Kubernetes versions. These steps need to be done manually before updating to the newest version of cass-operator. Newer Kubernetes versions require stricter validation and as such we need to remove ``preserveUnknownFields`` global property from the CRD to allow us to update to a newer CRD. The newer controller-runtime on the other hand modifies the liveness, readiness and configuration options, which require us to delete the older deployment. These commands do not delete your running Cassandra instances.

 Run the following commands assuming cass-operator is installed to ``cass-operator`` namespace (change -n parameter if it is installed to some other namespace):

```sh
kubectl -n cass-operator delete deployment.apps/cass-operator
kubectl -n cass-operator delete service/cassandradatacenter-webhook-service
kubectl patch crd cassandradatacenters.cassandra.datastax.com -p '{"spec":{"preserveUnknownFields":false}}'
```

You can now install new version of cass-operator as instructed previously.


## Supported versions

We actively maintain two different versions of cass-operator, the [1.10.x series](https://github.com/k8ssandra/cass-operator/tree/1.10.x) as well as the newest version cut from the master branch. The [1.10.x branch](https://github.com/k8ssandra/cass-operator/tree/1.10.x) receives only bugfixes, while all the new features will only land to the newer versions.

### Kubernetes and Openshift version support

1.10.x versions support Kubernetes versions 1.18 to 1.25. For Openshift, 1.10.x supports 4.7 and 4.8 (it will not pass validations in 4.9 due to the use of deprecated APIs).

Newer releases (1.11.0 and up) are tested and supported only in supported Kubernetes versions. That means versions 1.21 and up. For Openshift installations, 1.11.0 and up are supported in 4.9 and newer.

### Cassandra version support

For Cassandra versions, 1.10.x will only support 3.11.x and 4.0.x. For 4.1 and 5.0, we recommend using a build 1.17.0 or newer. Support for 4.1 will only land in upcoming versions (1.13.0 or later). Both versions support all DSE 6.8.x releases.

## Changelog between versions

The next major/minor version always includes changes from the previously released supported version, but no more than that. That is, if the patch release is released after the next (next in semver) major/minor, the patches are not integrated to it, but are repeated in the changelog for the next version if required (in some cases the bugfixes are not necessary for the next major/minor).

As an example, lets say the changelog has the following release sequence:

```
1.12.0
1.10.5
1.11.0
1.10.4
..
1.10.0
```

1.11.0 will include everything under it, including fixes from 1.10.4, however 1.11.0 does not have the fixes from 1.10.5 (as 1.11.0 was released before 1.10.5). 1.12.0 on the other hand will include all the fixes from 1.10.5, but those fixes are repeated as 1.12.0 lists everything changed from the previous major/minor, that is changes between 1.11.0 and 1.12.0.

While the 1.10.x branch will only have fixes released from that branch, the master branch will include all the versions.

## Creating a Cassandra cluster

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
kubectl apply -f https://raw.githubusercontent.com/k8ssandra/cass-operator/v1.22.1/operator/k8s-flavors/gke/storage.yaml
```

### Creating a CassandraDatacenter

The following resource defines a Cassandra 4.0.1 datacenter with 3 nodes on one rack, which you can also find at [config/samples/example-cassdc-three-nodes-single-rack.yaml](config/samples/example-cassdc-three-nodes-single-rack.yaml):

```yaml
apiVersion: cassandra.datastax.com/v1beta1
kind: CassandraDatacenter
metadata:
  name: dc1
spec:
  clusterName: development
  serverType: cassandra
  serverVersion: "4.0.1"
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
            storage: 10Gi
  resources:
    requests:
      memory: 2Gi
      cpu: 1000m
  podTemplateSpec:
    securityContext: {}
    containers:
      - name: cassandra
        securityContext: {}
  racks:
    - name: rack1
  config:
    jvm-server-options:
      initial_heap_size: "1G"
      max_heap_size: "1G"
    cassandra-yaml:
      num_tokens: 16
      authenticator: PasswordAuthenticator
      authorizer: CassandraAuthorizer
      role_manager: CassandraRoleManager

```

Apply the above as follows:

```console
kubectl -n cass-operator apply -f https://raw.githubusercontent.com/k8ssandra/cass-operator/master/config/samples/example-cassdc-three-nodes-single-rack.yaml
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
* For Cassandra 4.1 and up, we use [k8ssandra-client](https://github.com/k8ssandra/k8ssandra-client) to build the configuration.
* server-system-logger, a Vector agent for outputting Cassandra logs for kubectl. Implemented in [logger.Dockerfile](logger.Dockerfile)
* Cassandra/DSE, built from [datastax/management-api-for-apache-cassandra](https://github.com/k8ssandra/management-api-for-apache-cassandra),

The Cassandra container must be built with support for management-api, otherwise cass-operator will fail to work correctly.

### Overriding properties of cass-operator created containers

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

- Kubernetes cluster, 1.21 or newer. For Openshift, version 4.9 or newer. If you're using older versions, please install from [1.10.x branch](https://github.com/k8ssandra/cass-operator/tree/1.10.x), which supports Openshift 4.7 and Kubernetes 1.19.

## Contributing

If you wish to file a bug, enhancement proposal or have other questions, use the issues in this repository. We also accept PRs from the community.

For other means of contacting, check [k8ssandra community](https://k8ssandra.io/community/) resources.

### Developer setup

Almost every build, test, or development task requires the following pre-requisites...

* Golang 1.21 or newer
* Docker, either the docker.io packages on Ubuntu, Docker Desktop for Mac,
  or your preferred docker distribution. Other container engines such as podman should work also.
* Kind or similar Kubernetes distribution for testing (Docker Desktop / Minikube will work if correct StorageClass is added)

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

test target will spawn a envtest environment, which will require the ports to be available. To test the unit test workflow, one can use the [act](https://github.com/nektos/act) to run the tests with simply running `act -j testing`. While integration tests will work with `act` also, we do not recommend that, since it runs them serially and that takes a long time.

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

If you used the ``make deploy`` to deploy the operator, replace it with ``make undeploy`` to uninstall. With the `kubectl apply -k` option, replace it with `kubectl delete -k`.

## Contacts

For questions, please reach out on [k8ssandra Community](https://k8ssandra.io/community/) channels. For development questions, we are active on our Discord channel #k8ssandra-dev. Or you can open an issue.

## License

Copyright DataStax, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
