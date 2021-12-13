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

func createTask(command, namespace string) (types.NamespacedName, *api.CassandraTask) {
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

	Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

	return taskKey, task
}

func waitForTaskCompletion(taskKey types.NamespacedName) {
	Eventually(func() bool {
		emptyTask := &api.CassandraTask{}
		err := k8sClient.Get(context.TODO(), taskKey, emptyTask)
		Expect(err).ToNot(HaveOccurred())

		return emptyTask.Status.CompletionTime != nil && emptyTask.Status.Active == 0 && emptyTask.Status.Succeeded == 1
	}, time.Duration(5*time.Second)).Should(BeTrue())
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

		// TODO Cleanup shouldn't happen when cluster is created
		// TODO The additional seeds logging in the cass-pod'
		When("Running rebuild in datacenter", func() {
			It("Run a rebuild task against the datacenter pods", func() {
				By("Create a task for rebuild")
				taskKey, task := createTask("rebuild", testNamespaceName)

				waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/node/rebuild"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
			})
		})
		When("Running cleanup twice in the same datacenter", func() {
			It("Run a cleanup task against the datacenter pods", func() {
				By("Create a task for cleanup")
				taskKey, task := createTask("cleanup", testNamespaceName)

				waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))

				// This is hacky approach to run two jobs twice in the same test - resetting the callDetails
				callDetails.URLCounts = make(map[string]int)
				By("Create a task for second cleanup")
				taskKey, task = createTask("cleanup", testNamespaceName)

				waitForTaskCompletion(taskKey)

				Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(3))
				Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(BeNumerically(">=", 3))
				Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 3))

				verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
			})
		})
	})
	Context("Sync jobs", func() {
		var testNamespaceName string
		BeforeEach(func() {
			By("Create fake mgmt-api server")
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
			taskKey, task := createTask("cleanup", testNamespaceName)

			waitForTaskCompletion(taskKey)

			Expect(callDetails.URLCounts["/api/v0/ops/keyspace/cleanup"]).To(Equal(3))
			Expect(callDetails.URLCounts["/api/v0/ops/executor/job"]).To(Equal(0))
			Expect(callDetails.URLCounts["/api/v0/metadata/versions/features"]).To(BeNumerically(">", 1))

			verifyPodsHaveAnnotations(testNamespaceName, string(task.UID))
		})
	})
})
