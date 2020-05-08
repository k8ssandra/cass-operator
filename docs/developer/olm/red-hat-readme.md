<h2>Getting Started</h2>
<p>Quick start:</p>
At this time Operator Lifecycle Manager does not support the creation of <code>Secret</code>, <code>ValidatingWebhookConfiguration</code>, or <code>Service</code> resources as part of installation. When installing the DataStax Kubernetes Operator for Apache Cassandra these must be created externally before the operator will fully come online.

<pre>
  <code>
---
apiVersion: v1
data:
  tls.crt: ""
  tls.key: ""
kind: Secret
metadata:
  name: cass-operator-webhook-config
  namespace: test-operator
---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: cassandradatacenter-webhook-registration
webhooks:
- admissionReviewVersions:
  - v1beta1
  clientConfig:
    service:
      name: cassandradatacenter-webhook-service
      namespace: test-operator
      path: /validate-cassandra-datastax-com-v1beta1-cassandradatacenter
  failurePolicy: Ignore
  matchPolicy: Equivalent
  name: cassandradatacenter-webhook.cassandra.datastax.com
  rules:
  - apiGroups:
    - cassandra.datastax.com
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - cassandradatacenters
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  labels:
    name: cass-operator-webhook
  name: cassandradatacenter-webhook-service
  namespace: test-operator
spec:
  ports:
  - port: 443
    targetPort: 8443
  selector:
    name: cass-operator
  </code>
</pre>

<p>
  Note the namespace above must be adjusted to match the namespace where <code>cass-operator</code> is being deployed.
</p>
<p>
  With these resources in place the operator will install and start monitoring for <code>CassandraDatacenter</code> custom resources. Consider using the OpenShift console for creation of these resources or submitting a YAML file with <code>oc</code>.
</p>

<h2>Storage</h2>
<p>
  When running on OpenShift an appropriate <code>StorageClass</code> must be used. If your cluster currently does not provide a storage class consider creating your own with the <code>NoProvisioner</code> provisioner. This will prevent the dynamic creation of volumes and support manually created <code>PersistentVolumes</code>. For example:
</p>

<pre>
  <code>
---
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: no-provisioner
provisioner: kubernetes.io/no-provisioner
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
  </code>
</pre>

<p>
  From here you can create a persistent volume with the following syntax:
</p>

<pre>
  <code>
---
kind: PersistentVolume
apiVersion: v1
metadata:
  name: cass-operator-test-pv
spec:
  capacity:
    storage: 100Gi
  hostPath:
    path: /mnt/pv-data/cass-op-pv
    type: ''
  accessModes:
    - ReadWriteOnce
    - ReadWriteMany
    - ReadOnlyMany
  persistentVolumeReclaimPolicy: Recycle
  volumeMode: Filesystem
  storageClassName: no-provisioner
  </code>
</pre>

<p>
  Note the <code>storageClassName</code> matches the previously defined <code>StorageClass</code>.
</p>

<h3>Creating a CassandraDatacenter</h3>
<p>The following resource defines a Cassandra 3.11.6 datacenter with 3 nodes on one rack:</p>

<pre>
  <code>
---
apiVersion: cassandra.datastax.com/v1beta1
kind: CassandraDatacenter
metadata:
  name: dc1
spec:
  clusterName: cluster1
  serverType: cassandra
  serverVersion: 3.11.6
  managementApiAuth:
    insecure: {}
  size: 3
  storageConfig:
    cassandraDataVolumeClaimSpec:
      storageClassName: no-provisioner
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
  </code>
</pre>

<p>Apply the above as follows:</p>
<p><code>oc apply -f cassdc.yaml</code></p>

<p>You can check the status of pods in the Cassandra cluster as follows:</p>

<pre>
  <code>
$ kubectl -n cass-operator get pods --selector cassandra.datastax.com/cluster=cluster1
NAME READY STATUS RESTARTS AGE
cluster1-dc1-default-sts-0 2/2 Running 0 26h
cluster1-dc1-default-sts-1 2/2 Running 0 26h
cluster1-dc1-default-sts-2 2/2 Running 0 26h
  </code>
</pre>

<p>You can check to see the current progress of bringing the Cassandra datacenter online by checking the <code>cassandraOperatorProgress</code> field of the <code>CassandraDatacenter</code>'s <code>status</code> sub-resource as follows:</p>

<pre>
  <code>
$ kubectl -n cass-operator get cassdc/dc1 -o "jsonpath={.status.cassandraOperatorProgress}"
Ready
  </code>
</pre>

<p>(<code>cassdc</code> and <code>cassdcs</code> are supported short forms of <code>CassandraDatacenter</code>.)</p>
<p>A value of "Ready", as above, means the operator has finished setting up the Cassandra datacenter.</p>
<p>You can also check the Cassandra cluster status using <code>nodetool</code> by invoking it on one of the pods in the Cluster as follows:</p>

<pre>
  <code>
$ oc exec -it -c cassandra cluster1-dc1-default-sts-0 -- nodetool status
Datacenter: dc1
===============
Status=Up/Down
|/ State=Normal/Leaving/Joining/Moving/Stopped
-- Address Load Tokens Owns (effective) Host ID Rack
UN 10.233.105.125 224.82 KiB 1 65.4% 5e29b4c9-aa69-4d53-97f9-a3e26115e625 r1
UN 10.233.92.96 186.48 KiB 1 61.6% b119eae5-2ff4-4b06-b20b-c492474e59a6 r1
UN 10.233.90.54 205.1 KiB 1 73.1% 0a96e814-dcf6-48b9-a2ca-663686c8a495 r1
  </code>
</pre>

<h2>Features</h2>
<ul>
  <li>Proper token ring initialization, with only one node bootstrapping at a time</li>
  <li>Seed node management - one per rack, or three per datacenter, whichever is more</li>
  <li>Server configuration integrated into the CassandraDatacenter CRD</li>
  <li>Rolling reboot nodes by changing the CRD</li>
  <li>Store data in a rack-safe way - one replica per cloud AZ</li>
  <li>Scale up racks evenly with new nodes</li>
  <li>Replace dead/unrecoverable nodes</li>
  <li>Multi DC clusters (limited to one Kubernetes namespace)</li>
</ul>

<p>All features are documented in the <a href="https://github.com/k8ssandra/cass-operator/tree/master/docs/user">User Documentation</a>.</p>

<h3>Containers</h3>

<p>The operator is comprised of the following container images working in concert:</p>

<ul>
  <li>The operator, built from sources in the <a href="operator/">operator</a> directory.</li>
  <li>The config builder init container, built from sources in <a href="https://github.com/datastax/cass-config-builder">datastax/cass-config-builder</a>.</li>
  <li>Cassandra, built from <a href="https://github.com/datastax/management-api-for-apache-cassandra">datastax/management-api-for-apache-cassandra</a>, with Cassandra 3.11.6 support, and experimental support for Cassandra 4.0.0-alpha3.</li>
  <li>... or DSE, built from <a href="https://github.com/datastax/docker-images">datastax/docker-images</a>.</li>
</ul>

<h2>Requirements</h2>
<ul>
  <li>Kubernetes cluster, 1.12 or newer.</li>
  <li>Users who want to use a Kubernetes version from before 1.15 can use a manifest that supports x-preserve-unknown-fields on the CassandraDatacenter CRD - <a href="docs/user/cass-operator-manifests-pre-1.15.yaml">manifest</a></li>
</ul>

<h2>Contributing</h2>
<p>As of version 1.0, Cass Operator is maintained by a team at DataStax and it is part of what powers <a href="https://www.datastax.com/cloud/datastax-astra">DataStax Astra</a>. We would love for open source users to contribute bug reports, documentation updates, tests, and features.</p>

<h3>Developer setup</h3>
<p>Almost every build, test, or development task requires the following pre-requisites...</p>
<ul>
  <li>Golang 1.13</li>
  <li>Docker, either the docker.io packages on Ubuntu, Docker Desktop for Mac, or your preferred docker distribution.</li>
  <li><a href="https://magefile.org/">mage</a>: There are some tips for using mage in <a href="docs/developer/mage.md">docs/developer/mage.md</a></li>
</ul>

<h3>Building</h3>
<p>The operator uses <a href="https://magefile.org/">mage</a> for its build process.</p>

<h4>Build the Operator Container Image</h4>
<p>This build task will create the operator container image, building or rebuilding the binary from golang sources if necessary:</p>
<p><code>mage operator:buildDocker</code></p>

<h4>Build the Operator Binary</h4>
<p>If you wish to perform ONLY to the golang build or rebuild, without creating a container image:</p>
<p><code>mage operator:buildGo</code></p>

<h3>Testing</h3>
<p><code>mage operator:testGo</code></p>

<h4>End-to-end Automated Testing</h4>
<p>Run fully automated end-to-end tests...</p>
<p><code>mage integ:run</code></p>
<p>Docs about testing are <a href="tests/README.md">here</a>. These work against any k8s cluster with six or more worker nodes.</p>

<h4>Manual Local Testing</h4>
<p>There are a number of ways to run the operator, see the following docs for more information:</p>
<ul>
  <li><a href="docs/developer/kind.md">kind</a>: Kubernetes in Docker is the recommended Kubernetes distribution for use by software engineers working on the operator. KIND can simulate a k8s cluster with multiple worker nodes on a single physical machine, though it's necessary to dial down the database memory requests.</li>
</ul>
<p>The <a href="docs/user/README.md">user documentation</a> also contains information on spinning up your first operator instance that is useful regardless of what Kubernetes distribution you're using to do so.</p>

<h2>Not (Yet) Supported Features</h2>
<ul>
  <li>Cassandra:
    <ul>
      <li>Integrated data repair solution</li>
      <li>Integrated backup and restore solution</li>
    </ul>
  </li>
  <li>DSE:
    <ul>
      <li>Advanced Workloads: Analytics</li>
    </ul>
  </li>
</ul>

<h2>License</h2>
<p>Copyright DataStax, Inc.</p>
<p>Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at</p>
<p><a href="http://www.apache.org/licenses/LICENSE-2.0">http://www.apache.org/licenses/LICENSE-2.0</a></p>
<p>Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.</p>
