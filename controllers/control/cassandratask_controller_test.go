package control

import (
	"context"
	"net/http/httptest"

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

const (
	testDatacenterName = "dc1"
	testNamespaceName  = "test-task"
)

var _ = Describe("Execute jobs against all pods", func() {
	Context("Use the empty job", func() {
		It("Create necessary datacenter parts", func() {
			By("Create fake management API server")
			var err error
			var mockServer *httptest.Server
			mockServer, err = httphelper.CreateJobDetailsFakeServer()
			Expect(err).ToNot(HaveOccurred())
			mockServer.Start()
			// defer mockServer.Close()

			By("Create Datacenter, pods and set dc status to Ready")
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespaceName,
				},
			}
			Expect(k8sClient.Create(context.Background(), testNamespace)).Should(Succeed())

			// TODO Simplify and extract

			cassdcKey := types.NamespacedName{
				Name:      testDatacenterName,
				Namespace: testNamespaceName,
			}

			testDc := &cassdcapi.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name:        cassdcKey.Name,
					Namespace:   cassdcKey.Namespace,
					Annotations: map[string]string{},
				},
				Spec: cassdcapi.CassandraDatacenterSpec{
					ClusterName:   "test-dc",
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
					Name:      "test-cassdc-pod1",
					Namespace: testNamespaceName,
					Labels: map[string]string{
						cassdcapi.ClusterLabel:    "test-dc",
						cassdcapi.DatacenterLabel: "dc1",
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
		})
		It("Run a task against the datacenter pods", func() {
			By("Create a task to do nothing")
			taskKey := types.NamespacedName{
				Name:      "test-empty-task",
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
							Name:    "donothing-dc1",
							Command: "empty",
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

				return emptyTask.Status.CompletionTime != nil && emptyTask.Status.Active == 0 && emptyTask.Status.Succeeded == 1
			}).Should(BeTrue())

			podList := corev1.PodList{}
			Expect(k8sClient.List(context.TODO(), &podList, client.InNamespace(testNamespaceName))).To(Succeed())

			for _, pod := range podList.Items {
				Expect(pod.GetAnnotations()[podJobStatusAnnotation]).To(Equal(podJobCompleted))
			}
		})
	})
})
