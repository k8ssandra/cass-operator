package control

import (
	"context"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"time"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	api "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
)

func createDatacenter(dcName, namespace string) func() {
	return func() {
		By("Create Datacenter, pods and set dc status to Ready")
		clusterName := fmt.Sprintf("test-%s", dcName)
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

		testDc := &cassdcapi.CassandraDatacenter{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cassdcKey.Name,
				Namespace:   cassdcKey.Namespace,
				Annotations: map[string]string{},
			},
			Spec: cassdcapi.CassandraDatacenterSpec{
				ClusterName:   clusterName,
				ServerType:    "cassandra",
				ServerVersion: "4.0.1",
				Size:          3,
			},
			Status: cassdcapi.CassandraDatacenterStatus{},
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

		for i := 0; i < int(testDc.Spec.Size); i++ {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-cassdc-%s-pod%d", dcName, i),
					Namespace: namespace,
					Labels: map[string]string{
						cassdcapi.ClusterLabel:    clusterName,
						cassdcapi.DatacenterLabel: dcName,
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
	}
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
			Jobs: []api.CassandraJob{
				{
					Name:    fmt.Sprintf("%s-dc1", command),
					Command: command,
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
	}, time.Duration(5*time.Second)).Should(BeTrue())
	return emptyTask
}

func verifyPodsHaveAnnotations(testNamespaceName, taskId string) {
	podList := corev1.PodList{}
	Expect(k8sClient.List(context.TODO(), &podList, client.InNamespace(testNamespaceName))).To(Succeed())

	jobAnnotationKey := getJobAnnotationKey(taskId)
	for _, pod := range podList.Items {
		Expect(pod.GetAnnotations()).To(HaveKey(jobAnnotationKey))
	}
}

var _ = Describe("Execute jobs against all pods", func() {
	jobRunningRequeue = time.Duration(1 * time.Millisecond)
	Context("Async jobs", func() {
		var testNamespaceName string
		BeforeEach(func() {
			By("Create fake mgmt-api server")
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
		})

		When("Running rebuild in datacenter", func() {
			It("Run a rebuild task against the datacenter pods", func() {
				By("Create a task for rebuild")
				taskKey := createTask(api.CommandRebuild, testNamespaceName)

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/node/rebuild"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})
		})
		When("Running cleanup twice in the same datacenter", func() {
			It("Run a cleanup task against the datacenter pods", func() {
				By("Create a task for cleanup")
				taskKey := createTask(api.CommandCleanup, testNamespaceName)

				completedTask := waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))

				// This is hacky approach to run two jobs twice in the same test - resetting the callDetails
				callDetails.URLCounts = make(map[string]int)
				By("Create a task for second cleanup")
				taskKey = createTask("cleanup", testNamespaceName)

				completedTask = waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				// verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
				Expect(completedTask.Status.Succeeded).To(BeNumerically(">=", 1))
			})
		})
	})
	Context("Failing jobs", func() {
		When("In a datacenter", func() {
			It("Should fail once when no retryPolicy is set", func() {
				By("Create fake mgmt-api server")
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

				Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				Expect(completedTask.Status.Failed).To(BeNumerically(">=", 3))
			})
			It("If retryPolicy is set, we should see a retry", func() {
				By("Create fake mgmt-api server")
				callDetails := httphelper.NewCallDetails()
				mockServer, err := httphelper.FakeExecutorServerWithDetailsFails(callDetails)
				testFailedNamespaceName := fmt.Sprintf("test-task-failed-%d", rand.Int31())
				Expect(err).ToNot(HaveOccurred())
				mockServer.Start()
				defer mockServer.Close()

				By("create datacenter", createDatacenter("dc1", testFailedNamespaceName))
				By("Create a task for cleanup")
				taskKey, task := buildTask(api.CommandCleanup, testFailedNamespaceName)
				task.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

				completedTask := waitForTaskCompletion(taskKey)

				// Due to retry, we have double the amount of calls
				Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(6))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 6))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 6))

				Expect(completedTask.Status.Failed).To(BeNumerically(">=", 3))
			})
		})
	})
	Context("Sync jobs", func() {
		var testNamespaceName string
		BeforeEach(func() {
			By("Create fake synchronous mgmt-api server")
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
		})

		It("Run a cleanup task against the datacenter pods", func() {
			By("Create a task for cleanup")
			taskKey := createTask(api.CommandCleanup, testNamespaceName)

			completedTask := waitForTaskCompletion(taskKey)

			Expect(callDetails.URLCounts["/api/v0/ops/keyspace/cleanup"]).To(Equal(3))
			Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(Equal(0))
			Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

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
		It("Ensure task is deleted after TTL has expired", func() {
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
		It("Ensure task is not deleted if TTL is set to 0", func() {
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
