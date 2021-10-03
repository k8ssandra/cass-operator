# Kubernetes Kind cluster

For development purposes, the [Kind](https://kind.sigs.k8s.io/) cluster is the closest to a real Kubernetes cluster in terms of compatibility. For that reason, we use it as a default for our integration testing. We've built some tools and helpers to simplify the workflow for development purposes.

## Install kind

Some Linux distributions provide kind from their package registry, but in case you have Go already installed, this command is enough to install newest kind version:

```console
GO111MODULE="on" go get sigs.k8s.io/kind@v0.11.1
```

Or to download a binary directly:

```console
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.11.1/kind-linux-amd64
chmod +x ./kind
```

## Setting up a kind cluster

To start a new cluster, it is enough to run:

```console
kind cluster create
```

But some of tests require more than one Cassandra node (and more than one Kind worker node), so to start a six worker node cluster would be:

```console
kind cluster create --config=tests/testdata/kind/kind_config_6_workers.yaml
```

## Deploy cass-operator to kind cluster

To compile your version of cass-operator and load it to your kind cluster, run the following command:

```console
make docker-build docker-kind
```

That builds the operator and then loads the resulting image to your kind cluster. To deploy it:

```console
make deploy
```

And likewise, ``make undeploy`` will remove it from the kind cluster.

## Deleting the cluster

Running the following command deletes a kind cluster:

```console
kind delete cluster
```