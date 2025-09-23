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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

const (
	pollingTime = 50 * time.Millisecond
)

var testNamespaceName string

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

func createStorageClass(ctx context.Context, storageClassName string) {
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: storageClassName,
		},
		AllowVolumeExpansion: ptr.To(true),
		Provisioner:          "kubernetes.io/no-provisioner",
	}
	Expect(k8sClient.Create(ctx, sc)).To(Succeed())
}

func waitForDatacenterProgress(ctx context.Context, dcName string, state cassdcapi.ProgressState) AsyncAssertion {
	return Eventually(func(g Gomega) {
		dc := cassdcapi.CassandraDatacenter{}
		key := types.NamespacedName{Namespace: testNamespaceName, Name: dcName}

		g.Expect(k8sClient.Get(ctx, key, &dc)).To(Succeed())
		g.Expect(dc.Status.CassandraOperatorProgress).To(Equal(state))
	}).WithTimeout(20 * time.Second).WithPolling(pollingTime).WithContext(ctx)
}

func waitForDatacenterReady(ctx context.Context, dcName string) AsyncAssertion {
	return waitForDatacenterProgress(ctx, dcName, cassdcapi.ProgressReady)
}

func waitForDatacenterCondition(ctx context.Context, dcName string, condition cassdcapi.DatacenterConditionType, status corev1.ConditionStatus) AsyncAssertion {
	return Eventually(func(g Gomega) {
		dc := cassdcapi.CassandraDatacenter{}
		key := types.NamespacedName{Namespace: testNamespaceName, Name: dcName}

		g.Expect(k8sClient.Get(ctx, key, &dc)).To(Succeed())
		g.Expect(dc.Status.Conditions).ToNot(BeNil())
		for _, cond := range dc.Status.Conditions {
			if cond.Type == condition {
				g.Expect(cond.Status).To(Equal(status))
				return
			}
		}
		g.Expect(false).To(BeTrue(), "Condition not found")
	}).WithTimeout(20 * time.Second).WithPolling(pollingTime).WithContext(ctx)
}

var _ = Describe("CassandraDatacenter tests", func() {
	Describe("Creating a new datacenter", func() {
		BeforeEach(func() {
			testNamespaceName = fmt.Sprintf("test-cassdc-%d", rand.Int31())
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespaceName,
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).Should(Succeed())
			DeferCleanup(func() {
				// Note that envtest doesn't actually delete any namespace as it doesn't have kube-controller-manager running
				// but it does it mark it as "terminating", so modifying and adding new resources will cause an error in the test
				// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
				Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
			})
		})
		Context("Single rack basic operations", func() {
			It("should end up in a Ready state with a single node", func(ctx SpecContext) {
				dcName := "dc1"

				createDatacenter(ctx, dcName, 1, 1)
				waitForDatacenterReady(ctx, dcName).Should(Succeed())

				verifyStsCount(ctx, dcName, 1, 1).Should(Succeed())
				verifyPodCount(ctx, dcName, 1).Should(Succeed())

				waitForDatacenterCondition(ctx, dcName, cassdcapi.DatacenterReady, corev1.ConditionTrue).Should(Succeed())
				waitForDatacenterCondition(ctx, dcName, cassdcapi.DatacenterInitialized, corev1.ConditionTrue).Should(Succeed())

				deleteDatacenter(ctx, dcName)
				verifyDatacenterDeleted(ctx, dcName).Should(Succeed())
			})
			It("should end up in a Ready state with multiple nodes", func(ctx SpecContext) {
				dcName := "dc1"

				createDatacenter(ctx, dcName, 3, 1)

				waitForDatacenterReady(ctx, dcName).Should(Succeed())

				verifyStsCount(ctx, dcName, 1, 3).Should(Succeed())
				verifyPodCount(ctx, dcName, 3).Should(Succeed())

				deleteDatacenter(ctx, dcName)
				verifyDatacenterDeleted(ctx, dcName).Should(Succeed())
			})
			It("should be able to scale up", func(ctx SpecContext) {
				dcName := "dc1"

				dc := createDatacenter(ctx, dcName, 1, 1)
				waitForDatacenterReady(ctx, dcName).Should(Succeed())

				verifyStsCount(ctx, dcName, 1, 1).Should(Succeed())
				verifyPodCount(ctx, dcName, 1).Should(Succeed())

				refreshDatacenter(ctx, &dc)

				By("Updating the size to 3")
				patch := client.MergeFrom(dc.DeepCopy())
				dc.Spec.Size = 3
				Expect(k8sClient.Patch(ctx, &dc, patch)).To(Succeed())

				waitForDatacenterCondition(ctx, dcName, cassdcapi.DatacenterScalingUp, corev1.ConditionTrue).Should(Succeed())
				waitForDatacenterProgress(ctx, dcName, cassdcapi.ProgressUpdating).Should(Succeed())

				verifyStsCount(ctx, dcName, 1, 3).Should(Succeed())
				verifyPodCount(ctx, dcName, 3).Should(Succeed())

				waitForDatacenterReady(ctx, dcName).Should(Succeed())

				deleteDatacenter(ctx, dcName)
				verifyDatacenterDeleted(ctx, dcName).Should(Succeed())
			})
		})
		Context("There are multiple nodes in multiple racks", func() {
			It("should end up in a Ready state", func(ctx SpecContext) {
				dcName := "dc2"

				createDatacenter(ctx, dcName, 9, 3)
				waitForDatacenterReady(ctx, dcName).Should(Succeed())

				verifyStsCount(ctx, dcName, 3, 3).Should(Succeed())
				verifyPodCount(ctx, dcName, 9).Should(Succeed())

				deleteDatacenter(ctx, dcName)
				verifyDatacenterDeleted(ctx, dcName).Should(Succeed())
			})
		})
	})
})

func refreshDatacenter(ctx context.Context, dc *cassdcapi.CassandraDatacenter) {
	key := types.NamespacedName{Namespace: testNamespaceName, Name: dc.Name}
	Expect(k8sClient.Get(ctx, key, dc)).To(Succeed())
}

func verifyStsCount(ctx context.Context, dcName string, rackCount, podsPerSts int) AsyncAssertion {
	return Eventually(func(g Gomega) {
		stsAll := &appsv1.StatefulSetList{}
		g.Expect(k8sClient.List(ctx, stsAll, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		g.Expect(stsAll.Items).To(HaveLen(rackCount))

		for _, sts := range stsAll.Items {
			rackName := sts.Labels[cassdcapi.RackLabel]

			podList := &corev1.PodList{}
			g.Expect(k8sClient.List(ctx, podList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName, cassdcapi.RackLabel: rackName}, client.InNamespace(testNamespaceName))).To(Succeed())
			g.Expect(podList.Items).To(HaveLen(podsPerSts))
		}
	})
}

func verifyPodCount(ctx context.Context, dcName string, podCount int) AsyncAssertion {
	return Eventually(func(g Gomega) {
		podList := &corev1.PodList{}
		g.Expect(k8sClient.List(ctx, podList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		g.Expect(podList.Items).To(HaveLen(podCount))
	})
}

func verifyDatacenterDeleted(ctx context.Context, dcName string) AsyncAssertion {
	return Eventually(func(g Gomega) {
		// Envtest has no garbage collection, so we can only compare that the ownerReferences are correct and they would be GCed (for items which we do not remove)

		// Check that DC no longer exists
		dc := &cassdcapi.CassandraDatacenter{}
		dcKey := types.NamespacedName{Name: dcName, Namespace: testNamespaceName}
		err := k8sClient.Get(ctx, dcKey, dc)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())

		// Check that services would be autodeleted and then remove them
		svcList := &corev1.ServiceList{}
		g.Expect(k8sClient.List(ctx, svcList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		for _, svc := range svcList.Items {
			g.Expect(svc.OwnerReferences).To(HaveLen(1))
			verifyOwnerReference(g, svc.OwnerReferences[0], dcName)
			g.Expect(k8sClient.Delete(ctx, &svc)).To(Succeed())
		}

		// Check that all StS would be autoremoved and remove them
		stsAll := &appsv1.StatefulSetList{}
		g.Expect(k8sClient.List(ctx, stsAll, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		for _, sts := range stsAll.Items {
			g.Expect(sts.OwnerReferences).To(HaveLen(1))
			verifyOwnerReference(g, sts.OwnerReferences[0], dcName)
			g.Expect(k8sClient.Delete(ctx, &sts)).To(Succeed())
		}

		// Check that all PVCs were removed (we remove these)
		pvcList := &corev1.PersistentVolumeClaimList{}
		g.Expect(k8sClient.List(ctx, pvcList, client.MatchingLabels{cassdcapi.DatacenterLabel: dcName}, client.InNamespace(testNamespaceName))).To(Succeed())
		for _, pvc := range pvcList.Items {
			g.Expect(pvc.GetDeletionTimestamp()).ToNot(BeNil())
		}
	}).WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond)
}

func verifyOwnerReference(g Gomega, ownerRef metav1.OwnerReference, dcName string) {
	g.Expect(ownerRef.Kind).To(Equal("CassandraDatacenter"))
	g.Expect(ownerRef.Name).To(Equal(dcName))
	g.Expect(ownerRef.APIVersion).To(Equal("cassandra.datastax.com/v1beta1"))
}

func createStubCassDc(dcName string, nodeCount int32) cassdcapi.CassandraDatacenter {
	return cassdcapi.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dcName,
			Namespace: testNamespaceName,
			Annotations: map[string]string{
				cassdcapi.UpdateAllowedAnnotation: "true",
			},
		},
		Spec: cassdcapi.CassandraDatacenterSpec{
			ManagementApiAuth: cassdcapi.ManagementApiAuthConfig{
				Insecure: &cassdcapi.ManagementApiAuthInsecureConfig{},
			},
			ClusterName:   clusterName(),
			ServerType:    "cassandra",
			ServerVersion: "4.1.4",
			Size:          nodeCount,
			StorageConfig: cassdcapi.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To[string]("default"),
					AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
					Resources: corev1.VolumeResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{"storage": resource.MustParse("1Gi")},
					},
				},
			},
		},
		Status: cassdcapi.CassandraDatacenterStatus{},
	}
}
