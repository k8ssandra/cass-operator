package control

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"time"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	api "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	mockServer  *httptest.Server
	callDetails *httphelper.CallDetails
	// testNamespaceName  = ""
	testDatacenterName = "dc1"
	testDc             *cassdcapi.CassandraDatacenter
	clusterName        = ""
	nodeCount          = 9
	rackCount          = 3
)

func createDatacenter(dcName, namespace string) func() {
	return func() {
		By("Create Datacenter, pods and set dc status to Ready")
		clusterName = fmt.Sprintf("test-%s", dcName)
		testNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), testNamespace)).Should(Succeed())

		cassdcKey := types.NamespacedName{
			Name:      dcName,
			Namespace: namespace,
		}

		testDc = &cassdcapi.CassandraDatacenter{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cassdcKey.Name,
				Namespace:   cassdcKey.Namespace,
				Annotations: map[string]string{},
			},
			Spec: cassdcapi.CassandraDatacenterSpec{
				ClusterName:   clusterName,
				ServerType:    "cassandra",
				ServerVersion: "4.0.5",
				Size:          int32(nodeCount),
			},
			Status: cassdcapi.CassandraDatacenterStatus{},
		}

		testDc.Spec.Racks = make([]cassdcapi.Rack, 3)
		for i := 0; i < rackCount; i++ {
			testDc.Spec.Racks[i] = cassdcapi.Rack{
				Name: fmt.Sprintf("r%d", i),
			}
		}

		Expect(k8sClient.Create(context.Background(), testDc)).Should(Succeed())

		patchCassdc := client.MergeFrom(testDc.DeepCopy())
		testDc.Status.CassandraOperatorProgress = cassdcapi.ProgressReady
		testDc.Status.Conditions = []cassdcapi.DatacenterCondition{
			{
				Status: corev1.ConditionTrue,
				Type:   cassdcapi.DatacenterReady,
			},
		}
		Expect(k8sClient.Status().Patch(context.Background(), testDc, patchCassdc)).Should(Succeed())

		createStatefulSets(cassdcKey.Namespace)
		podsPerRack := nodeCount / rackCount
		for _, rack := range testDc.Spec.Racks {
			for j := 0; j < podsPerRack; j++ {
				createPod(namespace, clusterName, dcName, rack.Name, j)
			}
		}
	}
}

func createStatefulSets(namespace string) {
	podsPerRack := int32(nodeCount / rackCount)

	for _, rack := range testDc.Spec.Racks {
		name := fmt.Sprintf("%s-%s-%s-sts", clusterName, testDc.Name, rack.Name)
		stsKey := types.NamespacedName{Name: name, Namespace: namespace}
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stsKey.Name,
				Namespace: stsKey.Namespace,
				Labels:    testDc.GetRackLabels(rack.Name),
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: testDc.GetRackLabels(rack.Name),
				},
				Replicas:            &podsPerRack,
				ServiceName:         testDc.GetAllPodsServiceName(),
				PodManagementPolicy: appsv1.ParallelPodManagement,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: testDc.GetRackLabels(rack.Name),
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "cassandra",
								Image: "k8ssandra/cassandra-nothere:latest",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), sts)).Should(Succeed())

		Expect(k8sClient.Get(context.TODO(), stsKey, sts)).Should(Succeed())
		sts.Status.CurrentRevision = "0"
		Expect(k8sClient.Status().Update(context.TODO(), sts)).To(Succeed())
	}
}

func createPod(namespace, clusterName, dcName, rackName string, ordinal int) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s-sts-%d", clusterName, dcName, rackName, ordinal),
			Namespace: namespace,
			Labels: map[string]string{
				cassdcapi.ClusterLabel:    clusterName,
				cassdcapi.DatacenterLabel: dcName,
				cassdcapi.RackLabel:       rackName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "cassandra",
					Image: "k8ssandra/cassandra-nothere:latest",
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

	podIP := "127.0.0.1"

	patchPod := client.MergeFrom(pod.DeepCopy())
	pod.Status = corev1.PodStatus{
		PodIP: podIP,
		PodIPs: []corev1.PodIP{
			{
				IP: podIP,
			},
		},
	}
	Expect(k8sClient.Status().Patch(context.Background(), pod, patchPod)).Should(Succeed())
}

func deleteDatacenter(namespace string) {
	// Delete pods, statefulsets and dc
	Expect(k8sClient.DeleteAllOf(context.TODO(), &appsv1.StatefulSet{}, client.InNamespace(namespace))).Should(Succeed())
	Expect(k8sClient.DeleteAllOf(context.TODO(), &corev1.Pod{}, client.InNamespace(namespace))).Should(Succeed())
	Expect(k8sClient.DeleteAllOf(context.TODO(), testDc, client.InNamespace(namespace))).Should(Succeed())
}

func buildTask(command api.CassandraCommand, namespace string) (types.NamespacedName, *api.CassandraTask) {
	taskKey := types.NamespacedName{
		Name:      fmt.Sprintf("test-%s-task-%d", command, rand.Int31()),
		Namespace: namespace,
	}
	task := &api.CassandraTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskKey.Name,
			Namespace: taskKey.Namespace,
		},
		Spec: api.CassandraTaskSpec{
			Datacenter: corev1.ObjectReference{
				Name:      testDatacenterName,
				Namespace: namespace,
			},
			CassandraTaskTemplate: api.CassandraTaskTemplate{
				Jobs: []api.CassandraJob{
					{
						Name:    fmt.Sprintf("%s-dc1", command),
						Command: command,
					},
				},
			},
		},
		Status: api.CassandraTaskStatus{},
	}

	return taskKey, task
}

func createTask(command api.CassandraCommand, namespace string) types.NamespacedName {
	taskKey, task := buildTask(command, namespace)
	Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

	return taskKey
}

func waitForTaskCompletion(taskKey types.NamespacedName) *api.CassandraTask {
	var emptyTask *api.CassandraTask
	Eventually(func() bool {
		emptyTask = &api.CassandraTask{}
		err := k8sClient.Get(context.TODO(), taskKey, emptyTask)
		Expect(err).ToNot(HaveOccurred())

		return emptyTask.Status.CompletionTime != nil
	}, 5*time.Second).Should(BeTrue())
	return emptyTask
}

func waitForTaskFailed(taskKey types.NamespacedName) *api.CassandraTask {
	var emptyTask *api.CassandraTask
	Eventually(func() bool {
		emptyTask = &api.CassandraTask{}
		err := k8sClient.Get(context.TODO(), taskKey, emptyTask)
		Expect(err).ToNot(HaveOccurred())

		return emptyTask.Status.Failed > 0
	}, 5*time.Second).Should(BeTrue())
	return emptyTask
}

var _ = Describe("CassandraTask controller tests", func() {
	Describe("Execute jobs against all pods", func() {
		BeforeEach(func() {
			JobRunningRequeue = 1 * time.Millisecond
			TaskRunningRequeue = 1 * time.Millisecond
		})
		Context("Async jobs", func() {
			var testNamespaceName string
			BeforeEach(func() {
				By("Creating a fake mgmt-api server")
				var err error
				callDetails = httphelper.NewCallDetails()
				mockServer, err = httphelper.FakeExecutorServerWithDetails(callDetails)
				testNamespaceName = fmt.Sprintf("test-task-%d", rand.Int31())
				Expect(err).ToNot(HaveOccurred())
				mockServer.Start()
				By("create datacenter", createDatacenter(testDatacenterName, testNamespaceName))
			})

			AfterEach(func() {
				mockServer.Close()
				deleteDatacenter(testNamespaceName)
			})

			When("Running rebuild in datacenter", func() {
				It("Runs a rebuild task against the datacenter pods", func() {
					By("Creating a task for rebuild")
					taskKey := createTask(api.CommandRebuild, testNamespaceName)

					completedTask := waitForTaskCompletion(taskKey)

					Expect(callDetails.URLCounts["/api/v1/ops/node/rebuild"]).To(Equal(nodeCount))
					Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount))
					Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount))

					// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
					Expect(completedTask.Status.Succeeded).To(BeNumerically("==", nodeCount))

					Expect(completedTask.Status.Conditions).To(HaveLen(2))
					for _, cond := range completedTask.Status.Conditions {
						switch cond.Type {
						case string(api.JobComplete):
							Expect(cond.Status).To(Equal(metav1.ConditionTrue))
						case string(api.JobRunning):
							Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						}
					}
				})
			})
			It("Runs a UpgradeSSTables task against the datacenter pods", func() {
				By("Creating a task for upgradesstables")
				taskKey := createTask(api.CommandUpgradeSSTables, testNamespaceName)

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/tables/sstables/upgrade"]).To(Equal(nodeCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", nodeCount))
			})
			It("Runs a node move task against the datacenter pods", func() {
				By("Creating a task for move")

				taskKey, task := buildTask(api.CommandMove, testNamespaceName)
				pod1 := fmt.Sprintf("%s-%s-r0-sts-0", clusterName, testDatacenterName)
				pod2 := fmt.Sprintf("%s-%s-r1-sts-0", clusterName, testDatacenterName)
				pod3 := fmt.Sprintf("%s-%s-r2-sts-0", clusterName, testDatacenterName)
				task.Spec.Jobs[0].Arguments.NewTokens = map[string]string{
					pod1: "-123",
					pod2: "0",
					pod3: "123",
				}
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v0/ops/node/move"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", 3))
			})
			It("Runs a flush task against the datacenter pods", func() {
				By("Creating a task for flush")

				taskKey, task := buildTask(api.CommandFlush, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/tables/flush"]).To(Equal(nodeCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", nodeCount))
			})
			It("Runs a flush task against a pod", func() {
				By("Creating a task for flush")

				taskKey, task := buildTask(api.CommandFlush, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				task.Spec.Jobs[0].Arguments.PodName = fmt.Sprintf("%s-%s-r0-sts-0", clusterName, testDatacenterName)
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/tables/flush"]).To(Equal(1))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 1))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", 1))
			})

			It("Runs a garbagecollect task against the datacenter pods", func() {
				By("Creating a task for garbagecollect")
				taskKey, task := buildTask(api.CommandGarbageCollect, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/tables/garbagecollect"]).To(Equal(nodeCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", nodeCount))
			})

			It("Runs a garbagecollect task against rack's pods", func() {
				By("Creating a task for garbagecollect")
				taskKey, task := buildTask(api.CommandGarbageCollect, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				task.Spec.Jobs[0].Arguments.RackName = "r2"
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/tables/garbagecollect"]).To(Equal(nodeCount / rackCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount/rackCount))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount/rackCount))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", nodeCount/rackCount))
			})

			It("Runs a scrub task against a pod", func() {
				By("Creating a task for scrub")

				taskKey, task := buildTask(api.CommandScrub, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				task.Spec.Jobs[0].Arguments.PodName = fmt.Sprintf("%s-%s-r0-sts-0", clusterName, testDatacenterName)
				task.Spec.Jobs[0].Arguments.NoValidate = false
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/tables/scrub"]).To(Equal(1))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 1))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", 1))

				// Payloads should be of type ScrubRequest
				var sreq httphelper.ScrubRequest
				Expect(json.Unmarshal(callDetails.Payloads[0], &sreq)).Should(Succeed())
				Expect(sreq.CheckData).To(BeTrue())
				Expect(sreq.KeyspaceName).To(Equal("ks1"))
			})

			It("Runs a compaction task against a pod", func() {
				By("Creating a task for compaction")

				taskKey, task := buildTask(api.CommandCompaction, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				task.Spec.Jobs[0].Arguments.PodName = fmt.Sprintf("%s-%s-r0-sts-0", clusterName, testDatacenterName)
				task.Spec.Jobs[0].Arguments.Tables = []string{"table1"}
				task.Spec.Jobs[0].Arguments.SplitOutput = true

				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/tables/compact"]).To(Equal(1))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 1))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", 1))

				// Payloads should be of type CompactRequest
				var req httphelper.CompactRequest
				Expect(json.Unmarshal(callDetails.Payloads[0], &req)).Should(Succeed())
				Expect(req.KeyspaceName).To(Equal("ks1"))
				Expect(req.SplitOutput).To(BeTrue())
				Expect(req.Tables).To(HaveLen(1))
			})

			When("Running cleanup twice in the same datacenter", func() {
				It("Runs a cleanup task against the datacenter pods", func() {
					By("Creating a task for cleanup")
					taskKey := createTask(api.CommandCleanup, testNamespaceName)

					completedTask := waitForTaskCompletion(taskKey)

					Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(nodeCount))
					Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount))
					Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount))

					// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
					Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))

					// This is hacky approach to run two jobs twice in the same test - resetting the callDetails
					callDetails.URLCounts = make(map[string]int)
					By("Creating a task for second cleanup")
					taskKey = createTask("cleanup", testNamespaceName)

					completedTask = waitForTaskCompletion(taskKey)

					Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(nodeCount))
					Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount))
					Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount))

					// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
					Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
				})
			})
		})
		Context("Failing jobs", func() {
			When("In a datacenter", func() {
				It("Should fail once when no retryPolicy is set", func() {
					By("Creating fake mgmt-api server")
					callDetails := httphelper.NewCallDetails()
					mockServer, err := httphelper.FakeExecutorServerWithDetailsFails(callDetails)
					testFailedNamespaceName := fmt.Sprintf("test-task-failed-%d", rand.Int31())
					Expect(err).ToNot(HaveOccurred())
					mockServer.Start()
					defer mockServer.Close()

					By("create datacenter", createDatacenter("dc1", testFailedNamespaceName))
					By("Create a task for cleanup")
					taskKey := createTask(api.CommandCleanup, testFailedNamespaceName)

					completedTask := waitForTaskCompletion(taskKey)

					Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(nodeCount))
					Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", nodeCount))
					Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", nodeCount))

					Expect(completedTask.Status.Failed).To(BeNumerically("==", nodeCount))
					Expect(completedTask.Status.Conditions[2].Type).To(Equal(string(api.JobFailed)))
					Expect(completedTask.Status.Conditions[2].Message).To(Equal("any error"))
				})
				It("If retryPolicy is set, we should see a retry", func() {
					By("Creating fake mgmt-api server")
					callDetails := httphelper.NewCallDetails()
					mockServer, err := httphelper.FakeExecutorServerWithDetailsFails(callDetails)
					testFailedNamespaceName := fmt.Sprintf("test-task-failed-%d", rand.Int31())
					Expect(err).ToNot(HaveOccurred())
					mockServer.Start()
					defer mockServer.Close()

					By("create datacenter", createDatacenter("dc1", testFailedNamespaceName))
					By("Creating a task for cleanup")
					taskKey, task := buildTask(api.CommandCleanup, testFailedNamespaceName)
					task.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
					Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

					completedTask := waitForTaskCompletion(taskKey)

					// Due to retry, we try twice and then bail out
					Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(2 * nodeCount))
					Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 2*nodeCount))
					Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 2*nodeCount))

					Expect(completedTask.Status.Failed).To(BeNumerically("==", nodeCount))
					Expect(completedTask.Status.Conditions[2].Type).To(Equal(string(api.JobFailed)))
					Expect(completedTask.Status.Conditions[2].Message).To(Equal("any error"))
				})
				It("Replace a node in the datacenter without specifying the pod", func() {
					testFailedNamespaceName := fmt.Sprintf("test-task-failed-%d", rand.Int31())
					By("creating a datacenter", createDatacenter("dc1", testFailedNamespaceName))
					By("Creating a task for replacenode")
					taskKey, task := buildTask(api.CommandReplaceNode, testFailedNamespaceName)

					Expect(k8sClient.Create(context.TODO(), task)).Should(Succeed())

					completedTask := waitForTaskCompletion(taskKey)

					Expect(completedTask.Status.Failed).To(BeNumerically(">=", 1))
					Expect(completedTask.Status.Conditions[2].Type).To(Equal(string(api.JobFailed)))
					Expect(completedTask.Status.Conditions[2].Message).To(Equal("terminal error: valid pod_name to replace is required"))
				})
			})
		})
		Context("Sync jobs", func() {
			var testNamespaceName string
			BeforeEach(func() {
				By("Creating fake synchronous mgmt-api server")
				var err error
				callDetails = httphelper.NewCallDetails()
				mockServer, err = httphelper.FakeServerWithoutFeaturesEndpoint(callDetails)
				testNamespaceName = fmt.Sprintf("test-sync-task-%d", rand.Int31())
				Expect(err).ToNot(HaveOccurred())
				mockServer.Start()
				By("create datacenter", createDatacenter(testDatacenterName, testNamespaceName))
			})

			AfterEach(func() {
				mockServer.Close()
				deleteDatacenter(testNamespaceName)
			})

			It("Runs a cleanup task against the datacenter pods", func() {
				By("Creating a task for cleanup")
				taskKey := createTask(api.CommandCleanup, testNamespaceName)

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v0/ops/keyspace/cleanup"]).To(Equal(nodeCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(Equal(0))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})

			It("Runs a upgradesstables task against the datacenter pods", func() {
				By("Creating a task for upgradesstables")
				taskKey := createTask(api.CommandUpgradeSSTables, testNamespaceName)

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v0/ops/tables/sstables/upgrade"]).To(Equal(nodeCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(Equal(0))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})

			It("Replaces a node in the datacenter", func() {
				By("Creating a task for replacenode")
				taskKey, task := buildTask(api.CommandReplaceNode, testNamespaceName)

				podKey := types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s-r1-sts-%d", clusterName, testDatacenterName, 2),
					Namespace: testNamespaceName,
				}

				task.Spec.Jobs[0].Arguments.PodName = podKey.Name
				Expect(k8sClient.Create(context.TODO(), task)).Should(Succeed())

				// Verify the pod2 was deleted
				Eventually(func() bool {
					pod := &corev1.Pod{}
					err := k8sClient.Get(context.TODO(), podKey, pod)
					return err != nil && errors.IsNotFound(err)
				}, 3*time.Second).Should(BeTrue())

				// Recreate it so the process "finishes"
				createPod(testNamespaceName, clusterName, testDatacenterName, "r1", 2)

				completedTask := waitForTaskCompletion(taskKey)

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})

			It("Runs a flush task against the datacenter pods", func() {
				By("Creating a task for flush")
				taskKey, task := buildTask(api.CommandFlush, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v0/ops/tables/flush"]).To(Equal(nodeCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(Equal(0))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})

			It("Runs a garbagecollect task against the datacenter pods", func() {
				By("Creating a task for garbagecollect")
				taskKey, task := buildTask(api.CommandGarbageCollect, testNamespaceName)
				task.Spec.Jobs[0].Arguments.KeyspaceName = "ks1"
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v0/ops/tables/garbagecollect"]).To(Equal(nodeCount))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(Equal(0))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})
			It("Runs a ts reload task against a pod and fails", func() {
				By("Creating a task for tsreload")
				taskKey, task := buildTask(api.CommandTSReload, testNamespaceName)
				task.Spec.Jobs[0].Arguments.PodName = fmt.Sprintf("%s-%s-r0-sts-0", clusterName, testDatacenterName)
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskFailed(taskKey)

				Expect(callDetails.URLCounts["/api/v0/ops/node/encryption/internode/truststore/reload"]).To(Equal(0)) // This doesn't get called because the test of whether the feature exists doesn't pass. The features endpoint doesn't exist in this mock server.

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically("==", 0))
				Expect(completedTask.Status.Failed).To(BeNumerically(">", 0))
			})
		})
		Context("successful SyncFeature usage", func() {
			var testNamespaceName string
			BeforeEach(func() {
				By("Creating fake synchronous mgmt-api server")
				var err error
				callDetails = httphelper.NewCallDetails()
				mockServer, err = httphelper.FakeServerWithSyncFeaturesEndpoint(callDetails)
				testNamespaceName = fmt.Sprintf("test-sync-task-%d", rand.Int31())
				Expect(err).ToNot(HaveOccurred())
				mockServer.Start()
				By("create datacenter", createDatacenter(testDatacenterName, testNamespaceName))
			})
			It("Runs a ts reload task against a pod", func() {
				By("Creating a task for tsreload")
				taskKey, task := buildTask(api.CommandTSReload, testNamespaceName)
				task.Spec.Jobs[0].Arguments.PodName = fmt.Sprintf("%s-%s-r0-sts-0", clusterName, testDatacenterName)
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v0/ops/node/encryption/internode/truststore/reload"]).To(Equal(1))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 0)) // This doesn't get called because the test of whether the feature exists doesn't pass.

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})
		})
		Context("Task TTL", func() {
			var testNamespaceName string
			BeforeEach(func() {
				testNamespaceName = fmt.Sprintf("test-task-%d", rand.Int31())
				testNamespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespaceName,
					},
				}
				Expect(k8sClient.Create(context.Background(), testNamespace)).Should(Succeed())
			})
			It("Ensures task is deleted after TTL has expired", func() {
				taskKey, task := buildTask(api.CommandCleanup, testNamespaceName)
				metav1.SetMetaDataLabel(&task.ObjectMeta, taskStatusLabel, completedTaskLabelValue)
				ttlTime := new(int32)
				*ttlTime = 1
				task.Spec.TTLSecondsAfterFinished = ttlTime
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				timeNow := metav1.Now()
				task.Status.CompletionTime = &timeNow
				Expect(k8sClient.Status().Update(context.TODO(), task)).Should(Succeed())

				Eventually(func() bool {
					deletedTask := api.CassandraTask{}
					err := k8sClient.Get(context.TODO(), taskKey, &deletedTask)
					return err != nil && errors.IsNotFound(err)
				}).Should(BeTrue())
			})
			It("Ensures task is not deleted if TTL is set to 0", func() {
				taskKey, task := buildTask(api.CommandCleanup, testNamespaceName)
				metav1.SetMetaDataLabel(&task.ObjectMeta, taskStatusLabel, completedTaskLabelValue)
				ttlTime := new(int32)
				*ttlTime = 0
				task.Spec.TTLSecondsAfterFinished = ttlTime
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				timeNow := metav1.Now()
				task.Status.CompletionTime = &timeNow

				Expect(k8sClient.Status().Update(context.TODO(), task)).Should(Succeed())
				Consistently(func() bool {
					deletedTask := api.CassandraTask{}
					err := k8sClient.Get(context.TODO(), taskKey, &deletedTask)
					return err == nil
				}).Should(BeTrue())
			})
		})
	})
	Describe("Execute jobs against all StatefulSets", func() {
		var testNamespaceName string
		BeforeEach(func() {
			testNamespaceName = fmt.Sprintf("test-task-%d", rand.Int31())
			By("create datacenter", createDatacenter(testDatacenterName, testNamespaceName))
		})

		AfterEach(func() {
			deleteDatacenter(testNamespaceName)
		})

		Context("Restart", func() {
			It("Restarts a single rack", func() {
				stsKey := types.NamespacedName{Namespace: testNamespaceName, Name: fmt.Sprintf("%s-%s-r1-sts", clusterName, testDc.Name)}
				var sts appsv1.StatefulSet
				Expect(k8sClient.Get(context.TODO(), stsKey, &sts)).Should(Succeed())

				// Create task to restart r1
				taskKey, task := buildTask(api.CommandRestart, testNamespaceName)
				task.Spec.Jobs[0].Arguments.RackName = "r1"
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				Eventually(func() bool {
					Expect(k8sClient.Get(context.TODO(), stsKey, &sts)).Should(Succeed())
					_, found := sts.Spec.Template.ObjectMeta.Annotations[api.RestartedAtAnnotation]
					return found
				}).Should(BeTrue())

				Expect(k8sClient.Get(context.TODO(), taskKey, task)).To(Succeed())
				Expect(task.Status.CompletionTime).To(BeNil())

				// Imitate statefulset_controller
				Expect(k8sClient.Get(context.TODO(), stsKey, &sts)).Should(Succeed())
				sts.Status.UpdatedReplicas = sts.Status.Replicas
				sts.Status.ReadyReplicas = sts.Status.Replicas
				sts.Status.CurrentReplicas = sts.Status.Replicas
				sts.Status.UpdateRevision = "1"
				sts.Status.CurrentRevision = sts.Status.UpdateRevision
				sts.Status.ObservedGeneration = sts.GetObjectMeta().GetGeneration()

				Expect(k8sClient.Status().Update(context.TODO(), &sts)).Should(Succeed())

				// Set StatefulSet properties here so that the task completes.. verify first that there's been a change (but only to r1)
				_ = waitForTaskCompletion(taskKey)
				Expect(task.Status.Succeeded).Should(Equal(int(sts.Status.Replicas))) // Ensure that Succeeded field is updated.
				// Verify other racks haven't been modified
				var stsAll appsv1.StatefulSetList
				Expect(k8sClient.List(context.TODO(), &stsAll, client.MatchingLabels(map[string]string{cassdcapi.DatacenterLabel: testDc.Name}), client.InNamespace(testNamespaceName))).To(Succeed())
				Expect(stsAll.Items).To(HaveLen(rackCount))

				for _, sts := range stsAll.Items {
					if sts.Name == stsKey.Name {
						continue
					}
					_, found := sts.Spec.Template.Annotations[api.RestartedAtAnnotation]
					Expect(found).ToNot(BeTrue())
				}
			})
			It("Restarts datacenter", func() {
				var stsAll appsv1.StatefulSetList
				Expect(k8sClient.List(context.TODO(), &stsAll, client.MatchingLabels(map[string]string{cassdcapi.DatacenterLabel: testDc.Name}), client.InNamespace(testNamespaceName))).To(Succeed())
				Expect(stsAll.Items).To(HaveLen(rackCount))

				// Create task to restart all
				taskKey, task := buildTask(api.CommandRestart, testNamespaceName)
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				Eventually(func() bool {
					Expect(k8sClient.List(context.TODO(), &stsAll, client.MatchingLabels(map[string]string{cassdcapi.DatacenterLabel: testDc.Name}), client.InNamespace(testNamespaceName))).To(Succeed())

					inflight := 0

					for _, sts := range stsAll.Items {
						if _, found := sts.Spec.Template.ObjectMeta.Annotations[api.RestartedAtAnnotation]; found {
							if sts.Status.UpdateRevision == "" {
								inflight++
							}
						}
					}

					Expect(inflight).To(BeNumerically("<=", 1))

					for _, sts := range stsAll.Items {
						if _, found := sts.Spec.Template.ObjectMeta.Annotations[api.RestartedAtAnnotation]; found {
							// Imitate statefulset_controller
							if sts.Status.UpdateRevision != "1" {
								sts.Status.UpdatedReplicas = sts.Status.Replicas
								sts.Status.ReadyReplicas = sts.Status.Replicas
								sts.Status.CurrentReplicas = sts.Status.Replicas
								sts.Status.UpdateRevision = "1"
								sts.Status.CurrentRevision = sts.Status.UpdateRevision
								sts.Status.ObservedGeneration = sts.GetObjectMeta().GetGeneration()

								Expect(k8sClient.Status().Update(context.TODO(), &sts)).Should(Succeed())
							}
						} else if !found {
							return false
						}
					}
					return true
				}, "5s", "50ms").Should(BeTrue())

				_ = waitForTaskCompletion(taskKey)
			})
		})
	})
	Describe("Execute jobs against Datacenters", func() {
		var testNamespaceName string
		BeforeEach(func() {
			testNamespaceName = fmt.Sprintf("test-task-%d", rand.Int31())
			By("create datacenter", createDatacenter(testDatacenterName, testNamespaceName))
		})

		AfterEach(func() {
			deleteDatacenter(testNamespaceName)
		})

		Context("Refresh", func() {
			It("Adds an annotation if CassandraDatacenter does not have one and waits for completion", func() {
				taskKey, task := buildTask(api.CommandRefresh, testNamespaceName)
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				dc := &cassdcapi.CassandraDatacenter{}
				Eventually(func() bool {
					if err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: testDatacenterName, Namespace: testNamespaceName}, dc); err != nil {
						return false
					}
					if metav1.HasAnnotation(dc.ObjectMeta, cassdcapi.UpdateAllowedAnnotation) {
						return dc.Annotations[cassdcapi.UpdateAllowedAnnotation] == "once"
					}
					return false
				}, "5s", "50ms").Should(BeTrue())

				delete(dc.Annotations, cassdcapi.UpdateAllowedAnnotation)
				Expect(k8sClient.Update(context.Background(), dc)).Should(Succeed())

				_ = waitForTaskCompletion(taskKey)
			})
			It("Completes if autoupdate-spec is always allowed", func() {
				dc := &cassdcapi.CassandraDatacenter{}
				Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: testDatacenterName, Namespace: testNamespaceName}, dc)).To(Succeed())
				metav1.SetMetaDataAnnotation(&dc.ObjectMeta, cassdcapi.UpdateAllowedAnnotation, string(cassdcapi.AllowUpdateAlways))
				Expect(k8sClient.Update(context.Background(), dc)).Should(Succeed())

				taskKey, task := buildTask(api.CommandRefresh, testNamespaceName)
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())
				_ = waitForTaskCompletion(taskKey)
			})
		})
	})
})
