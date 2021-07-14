# cass-operator integration tests

[Ginkgo](https://onsi.github.io/ginkgo/) is used alongside kubectl for integration testing of the operator.

## Prerequisites
Install kubectl using the instructions [here](https://kubernetes.io/docs/tasks/tools/install-kubectl) 

The tests use Kustomize to deploy the cass-operator. Kustomize is installed if not present in the machine. We test against 4.1.x series of Kustomize at this point, but newer 3.x releases should work also. 

You will also need a running Kubernetes cluster. We use [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), but others should work also. Note that the storageClass is set to ``default`` in the tests.

## Running the tests
The tests themselves expect a running k8s cluster, with at least 6 worker nodes, and kubectl to be configured to point at the cluster.

### Running tests with custom operator image

If you wish to use a custom operator image (or deploy from custom container registry), modify the used image by running:

```bash
cd config/manager && kustomize edit set image controller=$(IMG)
```

Replace ``$(IMG)`` with the repository/image combination. One can also modify the ``tests/kustomize/kustomization.yaml`` if you wish modify the test scenario of deploying the operator.

### Kicking off the tests on an existing cluster
To kick off all integration tests against the cluster that your kubectl is currently configured against, run:

```
make integ-test
```

### Running a single test
If you only want to run a single test, you can specify it's parent directory in an environment variable called `M_INTEG_DIR`:

```
M_INTEG_DIR=scale_up make integ-test
```

## Test structure and design
Our tests are structured such that there is a single Ginkgo test suite
per integration test. This is done in order to provide the maximum amount
of flexibility when running locally and in Jenkins. The main benefits we
get from this structure are:

* We can separate the output streams from each test
* This allows for better parallelization
* It becomes trivial to run just a single test if needed

We also structure each test so that the steps live inside of a single spec.
This is done to support parallel test running, as well as halting the test on
failure.

After each test, we automatically clean up and delete the namespace that it used. 
It is important for tests to use a namespace that begins with `test-` in order for
cleanup logic to work correctly during CI builds.

### Logging/Debugging
When using the provided `NamespaceWrapper` struct and accompanying functions to
execute test steps, we automatically dump logs for the current namespace after
each step. This provides a snapshot in time for easy inspection of the cluster
state throughout the life of the test.

After the test is done running, whether it was successful or not, we do a
final log dump for the entire cluster.

By default, we dump logs out at:

```
build/kubectl_dump/<test name>/<date/time stamp>/<steps>
```
