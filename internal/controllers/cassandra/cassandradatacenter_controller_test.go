package controllers

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

var (
	testNamespaceName string
)

func clusterName() string {
	// TODO Modify when multiple clusters are needed
	return "cluster1"
}

func createDatacenter(ctx context.Context, dcName string, nodeCount, rackCount int) cassdcapi.CassandraDatacenter {
	testDc := createStubCassDc(dcName, int32(nodeCount))

	testDc.Spec.Racks = make([]cassdcapi.Rack, rackCount)
	for i := 0; i < rackCount; i++ {
		testDc.Spec.Racks[i] = cassdcapi.Rack{
			Name: fmt.Sprintf("r%d", i),
		}
	}

	Expect(k8sClient.Create(ctx, &testDc)).Should(Succeed())
	return testDc
}

func deleteDatacenter(ctx context.Context, dcName string) {
	dc := cassdcapi.CassandraDatacenter{}
	dcKey := types.NamespacedName{Name: dcName, Namespace: testNamespaceName}
	Expect(k8sClient.Get(ctx, dcKey, &dc)).To(Succeed())
	Expect(k8sClient.Delete(ctx, &dc)).To(Succeed())
}

func waitForDatacenterProgress(ctx context.Context, dcName string, state cassdcapi.ProgressState) {
	Eventually(func(g Gomega) {
		dc := cassdcapi.CassandraDatacenter{}
		key := types.NamespacedName{Namespace: testNamespaceName, Name: dcName}

		g.Expect(k8sClient.Get(ctx, key, &dc)).To(Succeed())
		g.Expect(dc.Status.CassandraOperatorProgress).To(Equal(state))
	}).WithTimeout(20 * time.Second).WithPolling(200 * time.Millisecond).WithContext(ctx).Should(Succeed())
}

func waitForDatacenterReady(ctx context.Context, dcName string) {
	waitForDatacenterProgress(ctx, dcName, cassdcapi.ProgressReady)
}

var _ = Describe("CassandraDatacenter tests", func() {
	Describe("Creating a new datacenter", func() {
		Context("Single datacenter", func() {
			BeforeEach(func() {
				testNamespaceName = fmt.Sprintf("test-cassdc-%d", rand.Int31())
				testNamespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespaceName,
					},
				}
				Expect(k8sClient.Create(context.Background(), testNamespace)).Should(Succeed())
			})

			AfterEach(func() {
				testNamespaceDel := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespaceName,
					},
				}
				Expect(k8sClient.Delete(context.TODO(), testNamespaceDel)).To(Succeed())
			})
			When("There is a single rack and a single node", func() {
				It("should end up in a Ready state", func(ctx SpecContext) {
					dcName := "dc1"

					createDatacenter(ctx, dcName, 1, 1)
					waitForDatacenterReady(ctx, dcName)

					verifyStsCount(ctx, dcName, 1, 1)
					verifyPodCount(ctx, dcName, 1)

					deleteDatacenter(ctx, dcName)
					verifyDatacenterDeleted(ctx, dcName)
				})
				It("should be able to scale up", func(ctx SpecContext) {
					dcName := "dc11"

					dc := createDatacenter(ctx, dcName, 1, 1)
					waitForDatacenterReady(ctx, dcName)

					verifyStsCount(ctx, dcName, 1, 1)
					verifyPodCount(ctx, dcName, 1)

					key := types.NamespacedName{Namespace: testNamespaceName, Name: dcName}
					Expect(k8sClient.Get(ctx, key, &dc)).To(Succeed())

					By("Updating the size to 3")
					dc.Spec.Size = 3
					Expect(k8sClient.Update(ctx, &dc)).To(Succeed())

					Eventually(func(g Gomega) {
						verifyStsCount(ctx, dcName, 1, 3)
						verifyPodCount(ctx, dcName, 3)
					})

					waitForDatacenterReady(ctx, dcName)

					deleteDatacenter(ctx, dcName)
					verifyDatacenterDeleted(ctx, dcName)
				})
			})
			When("There are multiple nodes in a single rack", func() {
				It("should end up in a Ready state", func(ctx SpecContext) {
					dcName := "dc2"

					createDatacenter(ctx, dcName, 3, 1)

					waitForDatacenterReady(ctx, dcName)

					verifyStsCount(ctx, dcName, 1, 3)
					verifyPodCount(ctx, dcName, 3)

					deleteDatacenter(ctx, dcName)
					verifyDatacenterDeleted(ctx, dcName)
				})
			})
			When("There are multiple nodes in multiple racks", func() {
				It("should end up in a Ready state", func(ctx SpecContext) {
					dcName := "dc3"

					createDatacenter(ctx, dcName, 9, 3)
					waitForDatacenterReady(ctx, dcName)

					verifyStsCount(ctx, dcName, 3, 3)
					verifyPodCount(ctx, dcName, 9)

					deleteDatacenter(ctx, dcName)
					verifyDatacenterDeleted(ctx, dcName)
				})
			})
		})
	})
})

func verifyStsCount(ctx context.Context, dcName string, rackCount, podsPerSts int) {
	Eventually(func(g Gomega) {
		stsAll := &appsv1.StatefulSetList{}
		g.Expect(k8sClient.List(ctx, stsAll, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		g.Expect(len(stsAll.Items)).To(Equal(rackCount))

		for _, sts := range stsAll.Items {
			rackName := sts.Labels[cassdcapi.RackLabel]

			podList := &corev1.PodList{}
			g.Expect(k8sClient.List(ctx, podList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName, cassdcapi.RackLabel: rackName}, client.InNamespace(testNamespaceName))).To(Succeed())
			g.Expect(len(podList.Items)).To(Equal(podsPerSts))
		}
	}).Should(Succeed())
}

func verifyPodCount(ctx context.Context, dcName string, podCount int) {
	Eventually(func(g Gomega) {
		podList := &corev1.PodList{}
		g.Expect(k8sClient.List(ctx, podList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		g.Expect(len(podList.Items)).To(Equal(podCount))
	}).Should(Succeed())
}

func verifyDatacenterDeleted(ctx context.Context, dcName string) {
	Eventually(func(g Gomega) {
		// Envtest has no garbage collection, so we can only compare that the ownerReferences are correct and they would be GCed (for items which we do not remove)

		// Check that DC no longer exists
		dc := &cassdcapi.CassandraDatacenter{}
		dcKey := types.NamespacedName{Name: dcName, Namespace: testNamespaceName}
		err := k8sClient.Get(ctx, dcKey, dc)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())

		// Check that services would be autodeleted
		svcList := &corev1.ServiceList{}
		g.Expect(k8sClient.List(ctx, svcList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		for _, svc := range svcList.Items {
			g.Expect(len(svc.OwnerReferences)).To(Equal(1))
			verifyOwnerReference(g, svc.OwnerReferences[0], dcName)
		}

		// Check that all StS would be autoremoved
		stsAll := &appsv1.StatefulSetList{}
		g.Expect(k8sClient.List(ctx, stsAll, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		for _, sts := range stsAll.Items {
			g.Expect(len(sts.OwnerReferences)).To(Equal(1))
			verifyOwnerReference(g, sts.OwnerReferences[0], dcName)
		}

		// Check that all PVCs were removed (we remove these)
		pvcList := &corev1.PersistentVolumeClaimList{}
		g.Expect(k8sClient.List(ctx, pvcList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		for _, pvc := range pvcList.Items {
			g.Expect(pvc.GetDeletionTimestamp()).ToNot(BeNil())
		}

	}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())
}

func verifyOwnerReference(g Gomega, ownerRef metav1.OwnerReference, dcName string) {
	g.Expect(ownerRef.Kind).To(Equal("CassandraDatacenter"))
	g.Expect(ownerRef.Name).To(Equal(dcName))
	g.Expect(ownerRef.APIVersion).To(Equal("cassandra.datastax.com/v1beta1"))
}

func createStubCassDc(dcName string, nodeCount int32) cassdcapi.CassandraDatacenter {
	return cassdcapi.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:        dcName,
			Namespace:   testNamespaceName,
			Annotations: map[string]string{},
		},
		Spec: cassdcapi.CassandraDatacenterSpec{
			ManagementApiAuth: cassdcapi.ManagementApiAuthConfig{
				Insecure: &cassdcapi.ManagementApiAuthInsecureConfig{},
			},
			ClusterName:   clusterName(),
			ServerType:    "cassandra",
			ServerVersion: "4.0.7",
			Size:          nodeCount,
			StorageConfig: cassdcapi.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: pointer.String("default"),
					AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
					Resources: corev1.ResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{"storage": resource.MustParse("1Gi")},
					},
				},
			},
		},
		Status: cassdcapi.CassandraDatacenterStatus{},
	}
}
