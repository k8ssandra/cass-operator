# Changelog

Changelog for Cass Operator, new PRs should update the `main / unreleased` section with entries in the order:

```markdown
* [CHANGE]
* [FEATURE]
* [ENHANCEMENT]
* [BUGFIX]
```

## unreleased

* [CHANGE] [#718](https://github.com/k8ssandra/cass-operator/issues/718) Update Kubernetes dependencies to 0.31.0 and controller-runtime to 0.19.x, remove controller-config and instead return command line options as the only way to modify controller-runtime options. cass-operator specific configuration will still remain in the OperatorConfig CRD. Removes kube-auth from the /metrics endpoint from our generated configs and instead adds network-policy generation as one optional module for Kustomize.
* [CHANGE] [#527](https://github.com/k8ssandra/cass-operator/issues/527) Migrate the Kustomize configuration to Kustomize 5 only. Support for using Kustomize 4.x to generate config is no longer supported.
* [CHANGE] [#757](https://github.com/k8ssandra/cass-operator/issues/757) Change starting sequence of pods to match StatefulSet controller's behavior. First start the last pod and move towards 0.
* [ENHANCEMENT] [#729](https://github.com/k8ssandra/cass-operator/issues/729) Modify NewMgmtClient to support additional transport option for the http.Client
* [ENHANCEMENT] [#737](https://github.com/k8ssandra/cass-operator/issues/737) Before issuing PVC deletion when deleting a datacenter, verify the PVCs that match the labels are not actually used by any pods.
* [ENHANCEMENT] [#750]((https://github.com/k8ssandra/cass-operator/issues/737) If Pod fails to start, it is moved to the last position in the starting sequence and some other pod is started first. This is to prevent a single failing pod from blocking the start of other pods.
* [BUGFIX] [#744](https://github.com/k8ssandra/cass-operator/issues/744) If StatefulSet was manually modified outside CassandraDatacenter, do not start such pods as they would need to be decommissioned instantly and could have IP conflict issues when doing so.

## v1.23.0

* [CHANGE] [#720](https://github.com/k8ssandra/cass-operator/issues/720) Always use ObjectMeta.Name for the PodDisruptionBudget resource name, not the DatacenterName
* [CHANGE] [#731](https://github.com/k8ssandra/cass-operator/issues/731) Make the concurrent restart of already bootstrapped nodes the default. If user wishes to revert to older behavior, set annotation ``cassandra.datastax.com/allow-parallel-starts: "false"`` to datacenter.
* [FEATURE] [#651](https://github.com/k8ssandra/cass-operator/issues/651) Add tsreload task for DSE deployments and ability to check if sync operation is available on the mgmt-api side
* [ENHANCEMENT] [#722](https://github.com/k8ssandra/cass-operator/issues/722) Add back the ability to track cleanup task before marking scale up as done. This is controlled by an annotation cassandra.datastax.com/track-cleanup-tasks
* [ENHANCEMENT] [#532](https://github.com/k8ssandra/k8ssandra-operator/issues/532) Extend ImageConfig type to allow additional parameters for k8ssandra-operator requirements. These include per-image PullPolicy / PullSecrets as well as additional image.  Also add ability to override a single HCD version image. 
* [ENHANCEMENT] [#636](https://github.com/k8ssandra/cass-operator/issues/636) Add support for new field in ImageConfig, imageNamespace. This will allow to override namespace of all images when using private registries. Setting it to empty will remove the namespace entirely.
* [BUGFIX] [#705](https://github.com/k8ssandra/cass-operator/issues/705) Ensure ConfigSecret has annotations map before trying to set a value

## v1.22.4

* [BUGFIX] [#708](https://github.com/k8ssandra/cass-operator/issues/708) Preserve existing SecurityContext when ReadOnlyRootFilesystem is set

## v1.22.3

* [FEATURE] [#701](https://github.com/k8ssandra/cass-operator/issues/701) Allow ReadOnlyRootFilesystem for DSE also with extra mounts to provide support for cass-config-builder setups

## v1.22.2

* [BUGFIX] [#703](https://github.com/k8ssandra/cass-operator/issues/703) Fix HCD config path from /etc/cassandra to /opt/hcd/resources/cassandra/conf

## v1.22.1

* [BUGFIX] [#687](https://github.com/k8ssandra/cass-operator/issues/687) Prevent a crash when when StorageClassName was not set in the CassandraDataVolumeClaimSpec
* [CHANGE] [#689](https://github.com/k8ssandra/cass-operator/issues/689) Allow k8ssandra.io labels and annotations in services config

## v1.22.0

* [FEATURE] [#263](https://github.com/k8ssandra/cass-operator/issues/263) Allow increasing the size of CassandraDataVolumeClaimSpec if the selected StorageClass supports it. This feature is currently behind a opt-in feature flag and requires an annotation ``cassandra.datastax.com/allow-storage-changes: true`` to be set in the CassandraDatacenter.
* [FEATURE] [#646](https://github.com/k8ssandra/cass-operator/issues/646) Allow starting multiple parallel pods if they have already previously bootstrapped and not planned for replacement. Set annotation ``cassandra.datastax.com/allow-parallel-starts: true`` to enable this feature.
* [ENHANCEMENT] [#648](https://github.com/k8ssandra/cass-operator/issues/648) Make MinReadySeconds configurable value in the Spec.
* [ENHANCEMENT] [#184](https://github.com/k8ssandra/cass-operator/issues/349) Add CassandraDatacenter.Status fields as metrics also
* [ENHANCEMENT] [#199](https://github.com/k8ssandra/cass-operator/issues/199) If .spec.readOnlyRootFilesystem is set, run the cassandra container with readOnlyRootFilesystem. Also, modify the default SecurityContext to mention runAsNonRoot: true
* [ENHANCEMENT] [#595](https://github.com/k8ssandra/cass-operator/issues/595) Update Vector to 0.39.0 and enforce the TOML file format in the starting command
* [BUGFIX] [#681](https://github.com/k8ssandra/cass-operator/issues/681) Remove nodes rights from the operator as it is not required

## v1.21.1

* [BUGFIX] [#665](https://github.com/k8ssandra/cass-operator/issues/665) The RequiresUpdate and ObservedGenerations were updated too often, even when the reconcile was not finished.

## v1.21.0

* [FEATURE] [#659](https://github.com/k8ssandra/cass-operator/issues/659) Add support for HCD serverType with versions 1.x.x. It will be deployed like Cassandra >= 4.1 for now.
* [FEATURE] [#660](https://github.com/k8ssandra/cass-operator/issues/660) Add support for DSE version 6.9.x and remove support for DSE 7.x.x. DSE 6.9 will be deployed like DSE 6.8.
* [ENHANCEMENT] [#663](https://github.com/k8ssandra/cass-operator/issues/663) Update k8ssandra-client for better Cassandra 5.0 config support
* [BUGFIX] [#652](https://github.com/k8ssandra/cass-operator/issues/652) Update nodeStatuses info when a pod is recreated
* [BUGFIX] [#656](https://github.com/k8ssandra/cass-operator/issues/656) After canary upgrade, it was not possible to continue upgrading rest of the nodes if the only change was removing the canary upgrade
* [BUGFIX] [#657](https://github.com/k8ssandra/cass-operator/issues/657) If a change did not result in StatefulSet changes, a Ready -> Ready state would lose an ObservedGeneration update in the status
* [BUGFIX] [#382](https://github.com/k8ssandra/cass-operator/issues/382) If StatefulSet has not caught up yet, do not allow any changes to it. Instead, requeue and wait (does not change behavior of forceRacksUpgrade)

## v1.20.0

* [CHANGE] [#566](https://github.com/k8ssandra/cass-operator/issues/566) BREAKING: StatefulSets will no longer be automatically updated if CassandraDatacenter is not modified, unless an annotation "cassandra.datastax.com/autoupdate-spec" is set to the CassandraDatacenter with value "always" or "once". This means users of config secret should set this variable to "always" to keep their existing behavior. For other users, this means that for example the upgrades of operator will no longer automatically apply updated settings or system-logger image. The benefit is that updating the operator no longer causes the cluster to have a rolling restart. A new condition to indicate such change could be necessary is called "RequiresUpdate" and it will be set to True until the next refresh of reconcile has happened. 
* [CHANGE] [#618](https://github.com/k8ssandra/cass-operator/issues/618) Update dependencies to support controller-runtime 0.17.2, modify required parts.
* [CHANGE] [#575](https://github.com/k8ssandra/cass-operator/issues/575) Allow modification of PodTemplateSpec.ObjectMeta.Labels (it will cause a rolling restart as StatefulSet controller can't apply them without a restart)
* [ENHANCEMENT] [#628](https://github.com/k8ssandra/cass-operator/issues/628) Replace pod task can replace any node, including those that have crashed
* [ENHANCEMENT] [#532](https://github.com/k8ssandra/cass-operator/issues/532) Instead of rejecting updates/creates with deprecated fields, return kubectl warnings.
* [ENHANCEMENT] [#637](https://github.com/k8ssandra/cass-operator/issues/637) Expand nodeStatuses to include IPs and racks.
* [BUGFIX] [#639](https://github.com/k8ssandra/cass-operator/issues/639) Remove InvalidState check, there's no need to block reconcile here. Keep the InvalidState as information for the user only.

## v1.19.1

* [BUGFIX] [#622](https://github.com/k8ssandra/cass-operator/issues/622) Fix sanitization of DC label
* [BUGFIX] [#626](https://github.com/k8ssandra/cass-operator/issues/626) If multiple datacenters are present in the same namespace, the seed-service ownership is constantly updated between these two resources

## v1.19.0

* [FEATURE] [#601](https://github.com/k8ssandra/cass-operator/pull/601) Add additionalAnnotations field to CR so that all resources created by the operator can be annotated.
* [BUGFIX] [#607](https://github.com/k8ssandra/cass-operator/issues/607) Add missing additional labels and annotations to the superuserSecret.
* [BUGFIX] [#612](https://github.com/k8ssandra/cass-operator/issues/612) Improve error message handling in the task jobs by retaining the message that previously failed pod has generated

## v1.18.2

* [BUGFIX] [#593](https://github.com/k8ssandra/cass-operator/issues/593) Update k8ssandra-client to 0.2.2 to fix the issue with clusterName config generation when using 4.1

## v1.18.1

## v1.18.0

* [CHANGE] [#479](https://github.com/k8ssandra/cass-operator/issues/479) Set the default deployed DSE image to use our newer management-api by changing the repository to datastax/dse-mgmtapi-6_8
* [CHANGE] [#573](https://github.com/k8ssandra/cass-operator/issues/573) Add the namespace as env variable in the server-system-logger container to label metrics with.
* [ENHANCEMENT] [#580](https://github.com/k8ssandra/cass-operator/issues/580) Add garbageCollect CassandraTask that removes deleted data
* [ENHANCEMENT] [#578](https://github.com/k8ssandra/cass-operator/issues/578) Add flush CassandraTask that flushed memtables to the disk
* [ENHANCEMENT] [#586](https://github.com/k8ssandra/cass-operator/issues/578) Add scrub CassandraTask that allows rebuilding SSTables
* [ENHANCEMENT] [#582](https://github.com/k8ssandra/cass-operator/issues/582) Add compaction CassandraTask to force a compaction
* [BUGFIX] [#585](https://github.com/k8ssandra/cass-operator/issues/585) If task validation fails, stop processing the task and mark the validation error to Failed condition

## v1.17.2

* [ENHANCEMENT] [#571](https://github.com/k8ssandra/cass-operator/issues/571) Ensure both "server-config-init" as well as "server-config-init-base" are always created in the initContainers if 4.1.x is used.  

## v1.17.1

* [CHANGE] [#541](https://github.com/k8ssandra/cass-operator/issues/541) Revert when deployed through OLM, add serviceAccount to Cassandra pods that use nonroot priviledge. This is no longer necessary with 1.17.0 and up.

## v1.17.0

* [CHANGE] [#565](https://github.com/k8ssandra/cass-operator/issues/565) Replace the use of wget with curl when making Kubernetes -> management-api HTTP(S) calls
* [CHANGE] [#553](https://github.com/k8ssandra/cass-operator/issues/553) dockerImageRunsAsCassandra is no longer used for anything as that's the default for current images. Use SecurityContext to override default SecurityContext (999:999)
* [ENHANCEMENT] [#554](https://github.com/k8ssandra/cass-operator/issues/554) Add new empty directory as mount to server-system-logger (/var/lib/vector) so it works with multiple securityContexes
* [ENHANCEMENT] [#512](https://github.com/k8ssandra/cass-operator/issues/512) Configs are now built using the newer k8ssandra-client config build command instead of the older cass-config-builder. This provides support for Cassandra 4.1.x config properties and to newer.
* [ENHANCEMENT] [#184](https://github.com/k8ssandra/cass-operator/issues/184) Implement metrics to indicate the status of the Datacenter pods. Experimental, these metric names and values could be changed in the future versions.

## v1.16.0

* [CHANGE] [#542](https://github.com/k8ssandra/cass-operator/issues/542) Support 7.x.x version numbers for DSE and 5.x.x for Cassandra
* [CHANGE] [#531](https://github.com/k8ssandra/cass-operator/issues/531) Update to Kubebuilder gov4-alpha layout structure
* [ENHANCEMENT] [#523](https://github.com/k8ssandra/cass-operator/issues/516) Spec.ServiceAccountName is introduced as replacements to Spec.ServiceAccount (to account for naming changes in Kubernetes itself), also PodTemplateSpec.Spec.ServiceAccountName is supported. Precendence order is: Spec.ServiceAccountName > Spec.ServiceAccount > PodTemplateSpec.
* [ENHANCEMENT] [#541](https://github.com/k8ssandra/cass-operator/issues/541) When deployed through OLM, add serviceAccount to Cassandra pods that use nonroot priviledge
* [CHANGE] [#516](https://github.com/k8ssandra/cass-operator/issues/516) Modify sidecar default CPU and memory limits.
* [CHANGE] [#495](https://github.com/k8ssandra/cass-operator/issues/495) Remove all the VMware PSP specific code from the codebase. This has been inoperational since 1.8.0
* [CHANGE] [#494](https://github.com/k8ssandra/cass-operator/issues/494) Remove deprecated generated clientsets. 

## v1.10.6

* [CHANGE] [#496](https://github.com/k8ssandra/cass-operator/issues/496) ScalingUp is no longer tied to the lifecycle of the cleanup job. The cleanup job is created after the ScaleUp has finished, but to track its progress one should check the status of the CassandraTask and not the CassandraDatacenter's status. Also, added a new annotation to the Datacenter "cassandra.datastax.com/no-cleanup", which if set prevents from the creation of the CassandraTask.
* [ENHANCEMENT] [#500](https://github.com/k8ssandra/cass-operator/issues/500) Allow the /start command to run for a longer period of time (up to 10 minutes), before killing the pod if no response is received. This is intermediate solution until we can correctly detect from the pod that the start is not proceeding correctly.
* [BUGFIX] [#444](https://github.com/k8ssandra/cass-operator/issues/444) Update cass-config-builder to 1.0.5.
* [BUGFIX] [#415](https://github.com/k8ssandra/cass-operator/issues/415) Fix version override + imageRegistry issue where output would be invalid
* [BUGFIX] [#437](https://github.com/k8ssandra/cass-operator/issues/437) Ignore cluster healthy check on Datacenter decommission. Rest of #437 fix is not applied since this version does not have that bug.
* [BUGFIX] [#404](https://github.com/k8ssandra/cass-operator/issues/404) Filter unallowed values from the rackname when used in Kubernetes resources
* [BUGFIX] [#455](https://github.com/k8ssandra/cass-operator/issues/455) After task had completed, the running state would still say true

## v1.15.0

* [CHANGE] [#501](https://github.com/k8ssandra/cass-operator/issues/501) Replaced server-system-logger with a Vector based implementation. Also, examples are added how the Cassandra system.log can be parsed to a more structured format.
* [CHANGE] [#496](https://github.com/k8ssandra/cass-operator/issues/496) ScalingUp is no longer tied to the lifecycle of the cleanup job. The cleanup job is created after the ScaleUp has finished, but to track its progress one should check the status of the CassandraTask and not the CassandraDatacenter's status. Also, added a new annotation to the Datacenter "cassandra.datastax.com/no-cleanup", which if set prevents from the creation of the CassandraTask.
* [ENHANCEMENT] [#500](https://github.com/k8ssandra/cass-operator/issues/500) Allow the /start command to run for a longer period of time (up to 10 minutes), before killing the pod if no response is received. This is intermediate solution until we can correctly detect from the pod that the start is not proceeding correctly.
* [BUGFIX] [#481](https://github.com/k8ssandra/cass-operator/issues/481) If CDC was enabled, disabling MCAC (old metric collector) was not possible

## v1.14.0

* [CHANGE] [#457](https://github.com/k8ssandra/cass-operator/issues/457) Extract task execution attributes into CassandraTaskTemplate
* [CHANGE] [#447](https://github.com/k8ssandra/cass-operator/issues/447) Update Github actions to remove all deprecated features (set-outputs, node v12 actions)
* [CHANGE] [#448](https://github.com/k8ssandra/cass-operator/issues/448) Update to operator-sdk 1.25.1, update to go 1.19, update to Kubernetes 1.25, remove amd64 restriction on local builds (cass-operator and system-logger will be built for aarch64 also)
* [CHANGE] [#442](https://github.com/k8ssandra/cass-operator/issues/442) Deprecate old internode-encryption storage mounts and cert generation. If old path /etc/encryption/node.jks is no longer present, then the storage mount is no longer created. For certificates with internode-encryption, we recommend using cert-manager.
* [CHANGE] [#329](https://github.com/k8ssandra/cass-operator/issues/329) Thrift port is no longer open for Cassandra 4.x installations
* [CHANGE] [#487](https://github.com/k8ssandra/cass-operator/pull/487) The AdditionalVolumes.PVCSpec is now a pointer. Also, webhook will allow modifying AdditionalVolumes. 
* [FEATURE] [#441](https://github.com/k8ssandra/cass-operator/issues/441) Implement a CassandraTask for moving single-token nodes
* [ENHANCEMENT] [#486](https://github.com/k8ssandra/cass-operator/issues/486) AdditionalVolumes accepts VolumeSource as the data also, allowing ConfigMap/Secret/etc to be mounted to cassandra container.
* [ENHANCEMENT] [#467](https://github.com/k8ssandra/cass-operator/issues/467) Add new metrics endpoint port (9000) to Cassandra container. This is used by the new mgmt-api /metrics endpoint. 
* [ENHANCEMENT] [#457](https://github.com/k8ssandra/cass-operator/issues/362) Allow overriding the datacenter name
* [ENHANCEMENT] [#476](https://github.com/k8ssandra/cass-operator/issues/476) Enable CDC for DSE deployments.
* [ENHANCEMENT] [#472](https://github.com/k8ssandra/cass-operator/issues/472) Add POD_NAME and NODE_NAME env variables that match metadata.name and spec.nodeName information
* [ENHANCEMENT] [#315](https://github.com/k8ssandra/cass-operator/issues/351) PodTemplateSpec allows setting Affinities, which are merged with the current rules. PodAntiAffinity behavior has changed, if allowMultipleWorkers is set to true the PodTemplateSpec antiAffinity rules are copied as is, otherwise merged with current restrictions. Prevent usage of deprecated rack.Zone (use topology.kubernetes.io/zone label instead), but allow removal of Zone.
* [BUGFIX] [#410](https://github.com/k8ssandra/cass-operator/issues/410) Fix installation in IPv6 only environment
* [BUGFIX] [#455](https://github.com/k8ssandra/cass-operator/issues/455) After task had completed, the running state would still say true
* [BUGFIX] [#488](https://github.com/k8ssandra/cass-operator/issues/488) Expose the new metrics port in services

## v1.13.1

* [CHANGE] [#291](https://github.com/k8ssandra/cass-operator/issues/291) Update Ginkgo to v2 (maintain current features, nothing additional from v2)
* [BUGFIX] [#431](https://github.com/k8ssandra/cass-operator/issues/431) Fix a bug where the restartTask would not provide success counts for restarted pods.
* [BUGFIX] [#444](https://github.com/k8ssandra/cass-operator/issues/444) Update cass-config-builder to 1.0.5. Update the target tag of cass-config-builder to :1.0 to allow future updates in 1.0.x without rolling restarts.
* [BUGFIX] [#437](https://github.com/k8ssandra/cass-operator/issues/437) Fix startOneNodeRack to not loop forever in case of StS with size 0 (such as decommission of DC)
* [CHANGE] [#442](https://github.com/k8ssandra/cass-operator/issues/442) Do not mount encryption-cred-storage or create internode CAs if not needed. Recommended approach is to use cert-manager instead of this old legacy method.

## v1.13.0

* [CHANGE] [#395](https://github.com/k8ssandra/cass-operator/issues/395) Make CassandraTask job arguments strongly typed
* [CHANGE] [#354](https://github.com/k8ssandra/cass-operator/issues/354) Remove oldDefunctLabel support since we recreate StS. Fix #335 created-by value to match expected value.
* [CHANGE] [#385](https://github.com/k8ssandra/cass-operator/issues/385) Deprecate CassandraDatacenter's RollingRestartRequested. Use CassandraTask instead.
* [CHANGE] [#397](https://github.com/k8ssandra/cass-operator/issues/397) Remove direct dependency to k8s.io/kubernetes
* [FEATURE] [#384](https://github.com/k8ssandra/cass-operator/issues/384) Add a new CassandraTask operation "replacenode" that removes the existing PVCs from the pod, deletes the pod and starts a replacement process.
* [FEATURE] [#387](https://github.com/k8ssandra/cass-operator/issues/387) Add a new CassandraTask operation "upgradesstables" that allows to do SSTable upgrades after Cassandra version upgrade.
* [ENHANCEMENT] [#385](https://github.com/k8ssandra/cass-operator/issues/385) Add rolling restart as a CassandraTask action.
* [ENHANCEMENT] [#398](https://github.com/k8ssandra/cass-operator/issues/398) Update to go1.18 builds, update to use Kubernetes 1.24 envtest + dependencies, operator-sdk 1.23, controller-gen 0.9.2, Kustomize 4.5.7, controller-runtime 0.12.2
* [ENHANCEMENT] [#383](https://github.com/k8ssandra/cass-operator/pull/383) Add UpgradeSSTables, Compaction and Scrub to management-api client. Improve CassandraTasks to have the ability to validate input parameters, filter target pods and do processing outside of pods.
* [ENHANCEMENT] [#381](https://github.com/k8ssandra/cass-operator/issues/381) Make bootstrap operations deterministic.
* [ENHANCEMENT] [#417](https://github.com/k8ssandra/cass-operator/issues/417) Allow loading imageConfig from byte array
* [BUGFIX] [#327](https://github.com/k8ssandra/cass-operator/issues/327) Replace node done through CassandraTask can replace a node that's stuck in the Starting state.
* [BUGFIX] [#404](https://github.com/k8ssandra/cass-operator/issues/404) Filter unallowed values from the rackname when used in Kubernetes resources
* [BUGFIX] [#415](https://github.com/k8ssandra/cass-operator/issues/415) Fix version override + imageRegistry issue where output would be invalid

## v1.12.0

* [CHANGE] [#370](https://github.com/k8ssandra/cass-operator/issues/370) If Cassandra start call fails, delete the pod
* [ENHANCEMENT] [#366](https://github.com/k8ssandra/cass-operator/issues/366) If no finalizer is present, do not process the deletion. To prevent cass-operator from re-adding the finalizer, add an annotation no-finalizer that prevents the re-adding.
* [ENHANCEMENT] [#360](https://github.com/k8ssandra/cass-operator/pull/360) If Datacenter quorum reports unhealthy state, change Status Condition DatacenterHealthy to False (DBPE-2283)
* [ENHANCEMENT] [#317](https://github.com/k8ssandra/cass-operator/issues/317) Add ability for users to define CDC settings which will cause an agent to start within the Cassandra JVM and pass mutation events from Cassandra back to a Pulsar broker. (Tested on OSS Cassandra 4.x only.)
* [ENHANCEMENT] [#369](https://github.com/k8ssandra/cass-operator/issues/369) Add configurable timeout for liveness / readiness and drain when mutual auth is used and a default timeout for all wget execs (required with mutual auth)
* [BUGFIX] [#335](https://github.com/k8ssandra/cass-operator/issues/335) Cleanse label values derived from cluster name, which can contain illegal chars. Include app.kubernetes.io/created-by label.
* [BUGFIX] [#330](https://github.com/k8ssandra/cass-operator/issues/330) Apply correct updates to Service labels and annotations through additionalServiceConfig (they are now validated and don't allow reserved prefixes).
* [BUGFIX] [#368](https://github.com/k8ssandra/cass-operator/issues/368) Do not fetch endpointStatus from pods that have not started
* [BUGFIX] [#364](https://github.com/k8ssandra/cass-operator/issues/364) Do not log any errors if we fail to get endpoint states from nodes.

## v1.10.5

* [CHANGE] [#370](https://github.com/k8ssandra/cass-operator/issues/370) If Cassandra start call fails, delete the pod
* [ENHANCEMENT] [#360](https://github.com/k8ssandra/cass-operator/pull/360) If Datacenter quorum reports unhealthy state, change Status Condition DatacenterHealthy to False (DBPE-2283)
* [BUGFIX] [#377](https://github.com/k8ssandra/cass-operator/issues/377) Add timeout to all calls made with wget (mutual-auth)
* [BUGFIX] [#355](https://github.com/k8ssandra/cass-operator/issues/335) Cleanse label values derived from cluster name, which can contain illegal chars.
* [BUGFIX] [#330](https://github.com/k8ssandra/cass-operator/issues/330) Apply correct updates to Service labels and annotations through additionalServiceConfig (they are now validated and don't allow reserved prefixes).
* [BUGFIX] [#368](https://github.com/k8ssandra/cass-operator/issues/368) Do not fetch endpointStatus from pods that have not started
* [BUGFIX] [#364](https://github.com/k8ssandra/cass-operator/issues/364) Do not log any errors if we fail to get endpoint states from nodes.

## v1.11.0

* [CHANGE] [#183](https://github.com/k8ssandra/cass-operator/issues/183) Move from PodDisruptionBudget v1beta1 to v1 (changes min. required Kubernetes version to 1.21)
* [ENHANCEMENT] [#325](https://github.com/k8ssandra/cass-operator/issues/325) Enable static checking (golangci-lint) in the repository and fix all the found issues.
* [ENHANCEMENT] [#292](https://github.com/k8ssandra/cass-operator/issues/292) Update to Go 1.17 with updates to dependencies: Kube 1.23.4 and controller-runtime 0.11.1

## v1.10.4

* [CHANGE] [#264](https://github.com/k8ssandra/cass-operator/issues/264) Generate PodTemplateSpec in CassandraDatacenter with metadata
* [BUGFIX] [#313](https://github.com/k8ssandra/cass-operator/issues/313) Remove Cassandra 4.0.x regexp restriction, allow 4.x.x
* [BUGFIX] [#322](https://github.com/k8ssandra/cass-operator/pull/322) Add missing requeue if decommissioned pods haven't been removed yet
* [BUGFIX] [#315](https://github.com/k8ssandra/cass-operator/pull/315) Validate podnames in the ReplaceNodes before moving them to NodeReplacements

## v1.10.3

* [FEATURE] [#309](https://github.com/k8ssandra/cass-operator/pull/309) If StatefulSets are modified in a way that they can't be updated directly, recreate them with new specs
* [ENHANCEMENT] [#312](https://github.com/k8ssandra/cass-operator/issues/312) Integration tests now output CassandraDatacenter and CassandraTask CRD outputs to build directory
* [BUGFIX] [#298](https://github.com/k8ssandra/cass-operator/issues/298) EndpointState has incorrect json key
* [BUGFIX] [#304](https://github.com/k8ssandra/cass-operator/issues/304) Hostname lookups on Cassandra pods fail
* [BUGFIX] [#311](https://github.com/k8ssandra/cass-operator/issues/311) Fix cleanup retry reconcile bug

## v1.10.2

Bundle tag only, no changes.

## v1.10.1

* [BUGFIX] [#278](https://github.com/k8ssandra/cass-operator/issues/278) ImageRegistry json key was incorrect in the definition type. Fixed from "imageRegistryOverride" to "imageRegistry"

## v1.10.0
* [CHANGE] [#271](https://github.com/k8ssandra/cass-operator/pull/271) Admission webhook's FailPolicy is set to Fail instead of Ignored
* [FEATURE] [#243](https://github.com/k8ssandra/cass-operator/pull/243) Task scheduler support to allow creating tasks that run for each pod in the cluster. The tasks have their own reconciliation process and lifecycle distinct from CassandraDatacenter as well as their own API package.
* [ENHANCEMENT] [#235](https://github.com/k8ssandra/cass-operator/issues/235) Adding AdditionalLabels to add on all resources managed by the operator
* [ENHANCEMENT] [#244](https://github.com/k8ssandra/cass-operator/issues/244) Add ability to skip Cassandra user creation
* [ENHANCEMENT] [#257](https://github.com/k8ssandra/cass-operator/issues/257) Add management-api client method to list schema versions
* [ENHANCEMENT] [#125](https://github.com/k8ssandra/cass-operator/issues/125) On delete, allow decommission of the datacenter if running in multi-datacenter cluster and cassandra.datastax.com/decommission-on-delete annotation is set on the CassandraDatacenter
* [BUGFIX] [#272](https://github.com/k8ssandra/cass-operator/issues/272) Strip password (if it has one) from CreateRole
* [BUGFIX] [#254](https://github.com/k8ssandra/cass-operator/pull/254) Safely set annotation on datacenter in config secret
* [BUGFIX] [#261](https://github.com/k8ssandra/cass-operator/issues/261) State of decommission can't be detected correctly if the RPC address is removed from endpoint state during decommission, but before it's finalized.

## v1.9.0
* [CHANGE] #202 Support fetching FeatureSet from management-api if available. Return RequestError with StatusCode when endpoint has bad status.
* [CHANGE] #213 Integration tests in Github Actions are now reusable across different workflows
* [CHANGE] Prevent instant requeues, every requeue must wait at least 500ms.
* [FEATURE] #193 Add new Management API endpoints to HTTP Helper: GetKeyspaceReplication, ListTables, CreateTable
* [FEATURE] #175 Add FQL reconciliation via parseFQLFromConfig and SetFullQueryLogging called from ReconcileAllRacks. CallIsFullQueryLogEnabledEndpoint and CallSetFullQueryLog functions to httphelper.
* [FEATURE] #233 Allow overriding default Cassandra and DSE repositories and give versions a default suffix
* [ENHANCEMENT] #185 Add more app.kubernetes.io labels to all the managed resources
* [ENHANCEMENT] #233 Simplify rebuilding the UBI images if vulnerability is found in the base images
* [ENHANCEMENT] #221 Improve bundle creation to be compatible with Red Hat's certified bundle rules
* [BUGFIX] #185 introduced a regression which caused labels to be updated in StatefulSet when updating a version. Keep the original as these are not allowed to be modified.
* [BUGFIX] #222 There were still occasions where clusterName was not an allowed value, cleaning them before creating StatefulSet
* [BUGFIX] #186 Run cleanups in the background once per pod and poll its state instead of looping endlessly

## v1.8.0

* [CHANGE] #178 If clusterName includes characters not allowed in the serviceName, strip those chars from service name.
* [CHANGE] #108 Integrate Fossa component/license scanning
* [CHANGE] #120 Removed Helm charts, use k8ssandra helm charts instead
* [CHANGE] #120 Deployment model is now Kustomize in cass-operator
* [CHANGE] #120 and #148 Package placements have been modified
* [CHANGE] #148 Project uses kubebuilder multigroup structure
* [CHANGE] #120 Mage has been removed and replaced with Makefile
* [CHANGE] #120 Webhook TLS certs require new deployment model (such as with cert-manager)
* [CHANGE] #145 SKIP_VALIDATING_WEBHOOK env variable is replaced with configuration in OperatorConfig
* [CHANGE] #161 Additional seeds service is always created, even if no additional seeds in the spec is defined
* [CHANGE] #163 If additional-seeds-service includes an IP address with targetRef, do not remove it
* [ENHANCEMENT] #173 Add ListKeyspace function to httphelper
* [ENHANCEMENT] #120 Update operator-sdk and modify the project to use newer kubebuilder v3 structure instead of v1
* [ENHANCEMENT] #145 Operaator can be configured with configuration objects from ConfigMap
* [ENHANCEMENT] #146 system-logger and cass-operator base is now UBI8-micro
* [ENHANCEMENT] #180 Add Kustomize components to improve installation experience and customization options
* [BUGFIX] #162 Affinity labels defined at rack-level should have precedence over DC-level ones
* [BUGFIX] #120 Fix ResourceVersion conflict in CheckRackPodTemplate
* [BUGFIX] #141 #110 Force update of HostId after pod replace process to avoid stale HostId
* [BUGFIX] #139 Bundle creation had some bugs after #120
* [BUGFIX] #134 Fix cluster-wide-installation to happen with Kustomize also, from config/cluster

## v1.7.1

* [BUGFIX] #103 Fix upgrade of StatefulSet, do not change service name

## v1.7.0

* [CHANGE] #1 Repository move
* [CHANGE] #19 Remove internode_encryption_test
* [CHANGE] #12 Remove Reaper sidecar integration
* [CHANGE] #8 Reduce dependencies of apis/CassandraDatacenter
* [FEATURE] #14 Override PodSecurityContext
* [FEATURE] #18 Allow DNS lookup by pod name
* [FEATURE] #27 Upgrade to Cassandra 4.0-RC1
* [FEATURE] #293 Add custom labels and annotations for services (see datastax/cass-operator#293)
* [FEATURE] #232 Use hostnames and DNS lookups for AdditionalSeeds (see datastax/cass-operator#293)
* [FEATURE] #28 Make tolerations configurable
* [FEATURE] #13 Provide server configuration with a secret
* [ENHANCEMENT] #7 Add CreateKeyspace and AlterKeyspace functions to httphelper
* [ENHANCEMENT] #20 Include max_direct_memory in examples
* [ENHANCEMENT] #16 Only build images once during integration tests
* [ENHANCEMENT] #1 Replace system-logger with a custom built image, enabling faster shutdown

## v1.6.0

Features:

* Upgrade to Go 1.14 [#347](https://github.com/datastax/cass-operator/commit/7d4a2451d22df762d1258b2de4a7711c3e6470d0)
* Add support for specifying additional PersistentVolumeClaims [#327](https://github.com/datastax/cass-operator/commit/8553579844b846e83131825444486c0f63d030fe)
* Add support for specify rack labels [#292](https://github.com/datastax/cass-operator/commit/61e286b959d886fd2fd1a572802d99efad8064ba)

Bug fixes:

* Retry decommission to prevent cluster from getting stuck in decommission state [#356](https://github.com/datastax/cass-operator/commit/4b8163c366155735b6f61af08623bb2edbfdf9ee)
* Set explicit tag for busybox image [#339](https://github.com/datastax/cass-operator/commit/56d6ea60589a32e89495e7961dce1b835751a5ed)
* Incorrect volume mounts are created when adding an init container with volume mounts
  [#309](https://github.com/datastax/cass-operator/commit/80852343858cd62b54e37146ada41f94efc3c85e)

Docs/tests:

* Introduce integration with [Go Report Card](https://goreportcard.com/) [#346](https://github.com/datastax/cass-operator/commit/694aebdaa88f09a5d9f0cc86e55e0a581a5beb1e)
* Add more examples for running integration tests [#338](https://github.com/datastax/cass-operator/commit/f033fbf24ed89f0469c04df75306dfeab4fdb175)

## v1.5.1

Bug fixes:

* Fixed reconciling logic in VMware k8s environments [#361](https://github.com/datastax/cass-operator/commit/821e58e6b283ef77b25ac7791ae9b60fbc1f69a1) [#342](https://github.com/datastax/cass-operator/commit/ddc3a99c1b38fb54ba293aeef74f878a3bb7e07e)
* Retry decommission if worker nodes are too slow to start it [#356](https://github.com/datastax/cass-operator/commit/475ea74eda629fa0038214dce5969a7f07d48615)

## v1.5.0

Features:

* Allow configuration of the system.log tail-er [#311](https://github.com/datastax/cass-operator/commit/14b1310164e57546cdf1995d44835b63f2dee471)
* Update Kubernetes support to cover 1.15 to 1.19 [#296](https://github.com/datastax/cass-operator/commit/a75754989cd29a38dd6406fdb75a5bad010f782d) [#304](https://github.com/datastax/cass-operator/commit/f34593199c06bb2536d0957eaff487153e36041a)
* Specify more named ports for advanced workloads, and additional ports for ClusterIP service [#289](https://github.com/datastax/cass-operator/commit/b0eae79dae3dc1466ace18b2008d3de81622db5d) [#291](https://github.com/datastax/cass-operator/commit/00c02be24779262a8b89f85e64eb02efb14e2d15)
* Support WATCH_NAMESPACE=* to watch all namespaces [#286](https://github.com/datastax/cass-operator/commit/f856de1cbeafee5fe4ad7154930d20ca743bd9cc)
* Support arbitrary Cassandra and DSE versions, using a regex for validation [#271](https://github.com/datastax/cass-operator/commit/71806eaa1c30e8555abeca9f4db12490e8658e1a)
* Support running cassandra as a non-root cassandra user [#275](https://github.com/datastax/cass-operator/commit/4006e56f6dc2ac2b7cfc4cc7012cad5ee50dbfe8)
* Always gracefully drain Cassandra nodes before pod termination, and make terminationGracePeriodSeconds configurable [#269](https://github.com/datastax/cass-operator/commit/3686fd112e1363b1d1160664d92c2a54b368906f)
* Add canaryUpgradeCount to support canary upgrade of a single node [#258](https://github.com/datastax/cass-operator/commit/f73041cc6f3bb3196b49ffaa615e4d1107350330)
* Support merging all user customizations into the Cassandra podTemplateSpec [#263](https://github.com/datastax/cass-operator/commit/b1851d9ebcd4087a210d8ee3f83b6e5a1ab49516)
* DSE 6.8.4 support [#257](https://github.com/datastax/cass-operator/commit/d4d4e3a49ece233457e0e1deebc9153241fe97be)
* Add arm64 support [#238](https://github.com/datastax/cass-operator/commit/f1f237ea6d3127264a014ad3df79d15964a01206)
* Support safely scaling down a datacenter, decommissioning Cassandra nodes [#242](https://github.com/datastax/cass-operator/commit/8d0516a7b5e7d1198cf8ed2de7b3968ef9e660af) [#265](https://github.com/datastax/cass-operator/commit/e6a420cf12c9402cc038f30879bc1ee8556f487a)
* Add support to override the default registry for all container images [#228](https://github.com/datastax/cass-operator/commit/f7ef6c81473841e42a9a581b44b1f33e1b4ab4c1)
* Better helm chart support for branch / master builds on private Docker registries [#236](https://github.com/datastax/cass-operator/commit/3516aa1993b06b405acf51476c327bec160070a1)
* Support for specific reconciliation logic in VMware k8s environments [#204](https://github.com/datastax/cass-operator/commit/e7d20fbb365bc7679856d2c84a1af70f73167b57) [#206](https://github.com/datastax/cass-operator/commit/a853063b9f08eb1a64cb248e23b5a4a5db92a5ac) [#203](https://github.com/datastax/cass-operator/commit/10ff0812aa0138a2eb04c588fbe625f6a69ce833) [#224](https://github.com/datastax/cass-operator/commit/b4d7ae4b0da149eb896bacfae824ed6547895dbb) [#259](https://github.com/datastax/cass-operator/commit/a90f03c17f3e9675c1ceff37a2a7c0c0c85950a1)  [#288](https://github.com/datastax/cass-operator/commit/c7c159d53d34b390e7966477b0fc2ed2f5ec333e)

Bug fixes:

* Increase default init container CPU for better startup performance [#261](https://github.com/datastax/cass-operator/commit/0eece8718b7a7a48868c260e88684c0f98cdc05b)
* Re-enable the most common quiet period [#253](https://github.com/datastax/cass-operator/commit/67217d8fc384652753331f5a1ae9cadd45ebc0cb)
* Added label to all-pods service to narrow metrics scrape selection [#277](https://github.com/datastax/cass-operator/commit/193afa5524e85a90cea84afe8167b356f67d365a)

Docs/tests:

* Add test for infinite reconcile [#220](https://github.com/datastax/cass-operator/commit/361ef2d5da59cd48fab7d4d58e3075051515b32c)
* Added keys from datastax/charts and updated README [#221](https://github.com/datastax/cass-operator/commit/5a56e98856d0c483eb4c415bd3c8b8292ffffe4b)
* Support integration tests with k3d v3 [#248](https://github.com/datastax/cass-operator/commit/06804bd0b3062900c10a4cad3d3124e83cf17bf0)
* Default to KIND for integration tests [#252](https://github.com/datastax/cass-operator/commit/dcaec731c5d0aefc2d2249c32592a275c42a46f0)
* Run oss/dse integration smoke tests in github workflow [#267](https://github.com/datastax/cass-operator/commit/08ecb0977e09b547c035e280b99f2c36fa1d5806)

## v1.4.1

Features:

* Use Cassandra 4.0-beta1 image [#237](https://github.com/datastax/cass-operator/commit/085cdbb1a50534d4087c344e1e1a7b28a5aa922e)
* Update to cass-config-builder 1.0.3 [#235](https://github.com/datastax/cass-operator/commit/7c317ecaa287f175e9556b48140b648d2ec60aa2)
* DSE 6.8.3 support [#233](https://github.com/datastax/cass-operator/commit/0b9885a654ab09bb4144934b217dbbc9d0869da3)

Bug fixes:

* Fix for enabling DSE advanced workloads [#230](https://github.com/datastax/cass-operator/commit/72248629b7de7944fe9e7e262ef8d3c7c838a8c6)

## v1.4.0

Features:

* Cassandra 3.11.7 support [#209](https://github.com/datastax/cass-operator/commit/ecf81573948cf239180ab62fa72b91c9a8354a4e)
* DSE 6.8.2 support [#207](https://github.com/datastax/cass-operator/commit/4d566f4a4d16c975919726b95a51c3be8729bf3e)
* Configurable resource requests and limits for init and system-logger containers. [#184](https://github.com/datastax/cass-operator/commit/94634ba9a4b04f33fe7dfee539d500e0d4a0c02f)
* Add quietPeriod and observedGeneration to the status [#190](https://github.com/datastax/cass-operator/commit/8cf67a6233054a6886a15ecc0e88bd6a9a2206bd)
* Update config builder init container to 1.0.2 [#193](https://github.com/datastax/cass-operator/commit/811bca57e862a2e25c1b12db71e0da29d2cbd454)
* Host network support [#186](https://github.com/datastax/cass-operator/commit/86b32ee3fc21e8cd33707890ff580a15db5d691b)
* Helm chart option for cluster-scoped install [#182](https://github.com/datastax/cass-operator/commit/6c23b6ffe0fd45fa299540ea494ebecb21bc4ac9)
* Create JKS for internode encryption [#156](https://github.com/datastax/cass-operator/commit/db8d16a651bc51c0256714ebdda2d51485c5d9fa)
* Headless ClusterIP service for additional seeds [#175](https://github.com/datastax/cass-operator/commit/5f7296295e4dd7fda6b7ce7e2faca2b9efe2e414)
* Operator managed NodePort service [#177](https://github.com/datastax/cass-operator/commit/b2cf2ecf05dfc240d670f13dffc8903fe14bb052)
* Experimental ability to run DSE advanced workloads [#158](https://github.com/datastax/cass-operator/commit/ade4246b811a644ace75f9f561344eb815d43d52)
* More validation logic in the webhook [#165](https://github.com/datastax/cass-operator/commit/3b7578987057fd9f90b7aeafea1d71ebbf691984)

Bug fixes:

* Fix watching CassDC to not trigger on status update [#212](https://github.com/datastax/cass-operator/commit/3ae79e01398d8f281769ef079bff66c3937eca24)
* Enumerate more container ports [#200](https://github.com/datastax/cass-operator/commit/b0c004dc02b22c34682a3602097c1e09b6261572)
* Resuming a stopped CassDC should not use the ScalingUp condition [#198](https://github.com/datastax/cass-operator/commit/7f26e0fd532ce690de282d1377dd00539ea8c251)
* Idiomatic usage of the term "internode" [#197](https://github.com/datastax/cass-operator/commit/62993fa113053075a772fe532c35245003912a2f)
* First-seed-in-the-DC logic should respect additionalSeeds [#180](https://github.com/datastax/cass-operator/commit/77750d11c62f2c3043f1088e377b743859c3be96)
* Use the additional seeds service in the config [#189](https://github.com/datastax/cass-operator/commit/4aaaff7b4e4ff4df626aa12f149329b866a06d35)
* Fix operator so it can watch multiple or all namespaces [#173](https://github.com/datastax/cass-operator/commit/bac509a81b6339219fe3fc313dbf384653563c59)

Docs/tests:

* Encryption documentation [#196](https://github.com/datastax/cass-operator/commit/3a139c5f717a165c0f32047b7813b103786132b8)
* Fix link to sample-cluster-sample-dc.yaml [#191](https://github.com/datastax/cass-operator/commit/28039ee40ac582074a522d2a13f3dfe15350caac)
* Kong Ingress Documentation [#160](https://github.com/datastax/cass-operator/commit/b70a995eaee988846e07f9da9b4ab07e443074c2)
* Adding AKS storage example [#164](https://github.com/datastax/cass-operator/commit/721056a435492e552cd85d18e38ea85569ba755f)
* Added ingress documentation and sample client application to docs [#140](https://github.com/datastax/cass-operator/commit/4dd8e7c5d53398bb827dda326762c2fa15c131f9)

## v1.3.0

* Add DSE 6.8.1 support, and update to config-builder 1.0.1 [#139](https://github.com/datastax/cass-operator/commit/8026d3687ee6eb783743ea5481ee51e69e284e1c)
* Experimental support for Cassandra Reaper running in sidecar mode [#116](https://github.com/datastax/cass-operator/commit/30ac85f3d71886b750414e90476c42394d439026)
* Support using RedHat universal base image containers [#95](https://github.com/datastax/cass-operator/commit/6f383bd8d22491c5a784611620e1327dafc3ffae)
* Provide an easy way to specify additional seeds in the CRD [#136](https://github.com/datastax/cass-operator/commit/0125b1f639830fad31f4b0b1b955ac991212fd16)
* Unblocking Kubernetes 1.18 support [#132](https://github.com/datastax/cass-operator/commit/b8bbbf15394119cbbd604aa40fdb9224a9f312cd)
* Bump version of Management API sidecar to 0.1.5 [#129](https://github.com/datastax/cass-operator/commit/248b30efe797d0656f2fc5c8e96dc3c431ab9a32)
* No need to always set LastRollingRestart status [#124](https://github.com/datastax/cass-operator/commit/d0635a2507080455ed252a26252a336a96252bc9)
* Set controller reference after updating StatefulSets, makes sure StatefulSets are cleaned up on delete [#121](https://github.com/datastax/cass-operator/commit/f90a4d30d37fa8ace8119dc7808fd7561df9270e)
* Use the PodIP for Management API calls [#112](https://github.com/datastax/cass-operator/commit/dbf0f67aa7c3831cd5dacc52b10b4dd1c59a32d1)
* Watch secrets to trigger reconciling user and password updates [#109](https://github.com/datastax/cass-operator/commit/394d25a6d6ec452ecd1667f3dca40b7496379eea)
* Remove NodeIP from status [#96](https://github.com/datastax/cass-operator/commit/71ed104a7ec642e13ef27bafb6ac1a6c0a28a21e)
* Add ability to specify additional Cassandra users in CassandraDatacenter [#94](https://github.com/datastax/cass-operator/commit/9b376e8be93976a0a344bcda2e417aa90dd9758f)
* Improve validation for webhook configuration [#103](https://github.com/datastax/cass-operator/commit/6656d1a2fd9cdec1fe495c28dd3fbac9617341f6)

## v1.2.0

* Support for several k8s versions in the helm chart [#97](https://github.com/datastax/cass-operator/commit/9d76ad8258aa4e1d4893a357546de7de80aef0a0)
* Ability to roll back a broken upgrade / configuration change [#85](https://github.com/datastax/cass-operator/commit/86b869df65f8180524dc12ff11502f6f6889eef5)
* Mount root as read-only and temp dir as memory emptyvol [#86](https://github.com/datastax/cass-operator/commit/0474057e8339da4f89b2e901ab697f10a2184d78)
* Fix managed-by label [#84](https://github.com/datastax/cass-operator/commit/39519b8bae8795542a5fb16a844aeb55cf3b2737)
* Add sequence diagrams [#90](https://github.com/datastax/cass-operator/commit/f1fe5fb3e07cec71a2ba0df8fabfec2b7751a95b)
* Add PodTemplateSpec in CassDC CRD spec, which allows defining a base pod template spec [#67](https://github.com/datastax/cass-operator/commit/7ce9077beab7fb38f0796c303c9e3a3610d94691)
* Support testing with k3d [#79](https://github.com/datastax/cass-operator/commit/c360cfce60888e54b10fdb3aaaa2e9521f6790cf)
* Add logging of all events for more reliable retrieval [#76](https://github.com/datastax/cass-operator/commit/3504367a5ac60f04724922e59cf86490ffb7e83d)
* Update to Operator SDK v0.17.0 [#78](https://github.com/datastax/cass-operator/commit/ac882984b78d9eb9e6b624ba0cfc11697ddfb3d2)
* Update Cassandra images to include metric-collector-for-apache-cassandra (MCAC) [#81](https://github.com/datastax/cass-operator/commit/4196f1173e73571985789e087971677f28c88d09)
* Run data cleanup after scaling up a datacenter [#80](https://github.com/datastax/cass-operator/commit/4a71be42b64c3c3bc211127d2cc80af3b69aa8e5)
* Requeue after the last node has its node-state label set to Started during cluster creation [#77](https://github.com/datastax/cass-operator/commit/b96bfd77775b5ba909bd9172834b4a56ef15c319)
* Remove delete verb from validating webhook [#75](https://github.com/datastax/cass-operator/commit/5ae9bee52608be3d2cd915e042b2424453ac531e)
* Add conditions to CassandraDatacenter status [#50](https://github.com/datastax/cass-operator/commit/8d77647ec7bfaddec7e0e616d47b1e2edb5a0495)
* Better support and safeguards for adding racks to a datacenter [#59](https://github.com/datastax/cass-operator/commit/0bffa2e8d084ac675e3e4e69da58c7546a285596)

## v1.1.0

* #27 Added a helm chart to ease installing.
* #23 #37 #46 Added a validating webhook for CassandraDatacenter.
* #43 Emit more events when reconciling a CassandraDatacenter.
* #47 Support `nodeSelector` to pin database pods to labelled k8s worker nodes.
* #22 Refactor towards less code listing pods.
* Several integration tests added.

## v1.0.0

* Project renamed to `cass-operator`.
* KO-281 Node replace added.
* KO-310 The operator will work to revive nodes that fail readiness for over 10 minutes
  by deleting pods.
* KO-317 Rolling restart added.
* K0-83 Stop the cluster more gracefully.
* KO-329 API version bump to v1beta1.

## v0.9.0

* KO-146 Create a secret for superuser creation if one is not provided.
* KO-288 The operator can provision Cassandra clusters using images from
  <https://github.com/datastax/management-api-for-apache-cassandra> and the primary
  CRD the operator works on is a `v1alpha2` `cassandra.datastax.com/CassandraDatacenter`
* KO-210 Certain `CassandraDatacenter` inputs were not rolled out to pods during
  rolling upgrades of the cluster. The new process considers everything in the
  statefulset pod template.
* KO-276 Greatly improved integration tests on real KIND / GKE Kubernetes clusters
  using Ginkgo.
* KO-223 Watch fewer Kubernetes resources.
* KO-232 Following best practices for assigning seed nodes during cluster start.
* KO-92 Added a container that tails the system log.

## v0.4.1

* KO-190 Fix bug introduced in v0.4.0 that prevented scaling up or deleting
  datacenters.
* KO-177 Create a headless service that includes pods that are not ready. While
  this is not useful for routing CQL traffic, it can be helpful for monitoring
  infrastructure like Prometheus that would like to attempt to collect metrics
  from pods even if they are unhealthy, and which can tolerate connection
  failure.

## v0.4.0

* KO-97  Faster cluster deployments
* KO-123 Custom CQL super user. Clusters can now be provisioned without the
  publicly known super user `cassandra` and publicly known default password
  `cassandra`.
* KO-42  Preliminary support for DSE upgrades
* KO-87  Preliminary support for two-way SSL authentication to the DSE
  management API. At this time, the operator does not automatically create
  certificates.
* KO-116 Fix pod disruption budget calculation. It was incorrectly calculated
  per-rack instead of per-datacenter.
* KO-129 Provide `allowMultipleNodesPerWorker` parameter to enable testing
  on small k8s clusters.
* KO-136 Rework how DSE images and versions are specified.

## v0.3.0

* Initial labs release.
