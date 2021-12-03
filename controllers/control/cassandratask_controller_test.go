package control

import (
	"context"
	"fmt"
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
				Size:          1,
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

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-cassdc-%s-pod1", dcName),
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

var _ = Describe("Execute jobs against all pods", func() {
	Context("Async jobs", func() {
		BeforeEach(func() {
			var err error
			callDetails := httphelper.NewCallDetails()
			mockServer, err = httphelper.FakeExecutorServerWithDetails(callDetails)
			Expect(err).ToNot(HaveOccurred())
			mockServer.Start()
		})

		AfterEach(func() {
			mockServer.Close()
		})

		testNamespaceName := "test-task-async-cleanup"
		testDatacenterName := "dc1"

		It("Create necessary datacenter parts", createDatacenter(testDatacenterName, testNamespaceName))
		It("Run a cleanup task against the datacenter pods", func() {
			By("Create a task for cleanup")
			taskKey := types.NamespacedName{
				Name:      "test-cleanup-task",
				Namespace: testNamespaceName,
			}
			task := &api.CassandraTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskKey.Name,
					Namespace: taskKey.Namespace,
				},
				Spec: api.CassandraTaskSpec{
					Datacenter: corev1.ObjectReference{
						Name:      testDatacenterName,
						Namespace: testNamespaceName,
					},
					Jobs: []api.CassandraJob{
						{
							Name:    "cleanup-dc1",
							Command: "cleanup",
						},
					},
				},
				Status: api.CassandraTaskStatus{},
			}
			Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

			Eventually(func() bool {
				emptyTask := &api.CassandraTask{}
				err := k8sClient.Get(context.TODO(), taskKey, emptyTask)
				Expect(err).ToNot(HaveOccurred())

				// fmt.Printf("Details: %v ; %d ; %d\n", emptyTask.Status.CompletionTime, emptyTask.Status.Active, emptyTask.Status.Succeeded)
				return emptyTask.Status.CompletionTime != nil && emptyTask.Status.Active == 0 && emptyTask.Status.Succeeded == 1
			}, time.Duration(5*time.Second)).Should(BeTrue())

			// TODO Add CallDetails checks here
			// Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(1))

			podList := corev1.PodList{}
			Expect(k8sClient.List(context.TODO(), &podList, client.InNamespace(testNamespaceName))).To(Succeed())

			for _, pod := range podList.Items {
				Expect(pod.GetAnnotations()[podJobStatusAnnotation]).To(Equal(podJobCompleted))
			}
		})
		It("Run a rebuild task against the datacenter pods", func() {
			By("Create a task for cleanup")
			taskKey := types.NamespacedName{
				Name:      "test-rebuild-task",
				Namespace: testNamespaceName,
			}
			task := &api.CassandraTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskKey.Name,
					Namespace: taskKey.Namespace,
				},
				Spec: api.CassandraTaskSpec{
					Datacenter: corev1.ObjectReference{
						Name:      testDatacenterName,
						Namespace: testNamespaceName,
					},
					Jobs: []api.CassandraJob{
						{
							Name:    "rebuild-dc1",
							Command: "rebuild",
						},
					},
				},
				Status: api.CassandraTaskStatus{},
			}
			Expect(k8sClient.Create(context.Background(), task)).Should(Succeed())

			Eventually(func() bool {
				emptyTask := &api.CassandraTask{}
				err := k8sClient.Get(context.TODO(), taskKey, emptyTask)
				Expect(err).ToNot(HaveOccurred())

				// fmt.Printf("Details: %v ; %d ; %d\n", emptyTask.Status.CompletionTime, emptyTask.Status.Active, emptyTask.Status.Succeeded)
				return emptyTask.Status.CompletionTime != nil && emptyTask.Status.Active == 0 && emptyTask.Status.Succeeded == 1
			}, time.Duration(5*time.Second)).Should(BeTrue())

			// TODO Add CallDetails checks here
			// Expect(callDetails.URLCounts["/api/v1/ops/keyspace/cleanup"]).To(Equal(1))

			/*
				podList := corev1.PodList{}
				Expect(k8sClient.List(context.TODO(), &podList, client.InNamespace(testNamespaceName))).To(Succeed())

				for _, pod := range podList.Items {
					Expect(pod.GetAnnotations()[podJobStatusAnnotation]).To(Equal(podJobCompleted))
				}
			*/
		})
	})
	Context("Sync jobs", func() {
		BeforeEach(func() {
			var err error
			callDetails := httphelper.NewCallDetails()
			mockServer, err = httphelper.FakeServerWithoutFeaturesEndpoint(callDetails)
			Expect(err).ToNot(HaveOccurred())
			mockServer.Start()
		})

		AfterEach(func() {
			mockServer.Close()
		})

		testNamespaceName := "test-task-sync-cleanup"
		testDatacenterName := "dc1"

		It("Create necessary datacenter parts", createDatacenter(testDatacenterName, testNamespaceName))
	})
})
