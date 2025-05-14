// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"k8s.io/utils/ptr"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	taskapi "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_validateLabelsForCluster(t *testing.T) {
	type args struct {
		resourceLabels map[string]string
		rc             *ReconciliationContext
	}
	tests := []struct {
		name       string
		args       args
		want       bool
		wantLabels map[string]string
	}{
		{
			name: "No labels",
			args: args{
				resourceLabels: make(map[string]string),
				rc: &ReconciliationContext{
					Datacenter: &api.CassandraDatacenter{
						ObjectMeta: metav1.ObjectMeta{
							Name: "exampleDC",
						},
						Spec: api.CassandraDatacenterSpec{
							ClusterName:   "exampleCluster",
							ServerVersion: "4.0.1",
						},
					},
				},
			},
			want: true,
			wantLabels: map[string]string{
				api.ClusterLabel:        "exampleCluster",
				oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
				oplabels.NameLabel:      oplabels.NameLabelValue,
				oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
				oplabels.InstanceLabel:  fmt.Sprintf("%s-exampleCluster", oplabels.NameLabelValue),
				oplabels.VersionLabel:   "4.0.1",
			},
		}, {
			name: "Cluster name with spaces",
			args: args{
				resourceLabels: make(map[string]string),
				rc: &ReconciliationContext{
					Datacenter: &api.CassandraDatacenter{
						ObjectMeta: metav1.ObjectMeta{
							Name: "exampleDC",
						},
						Spec: api.CassandraDatacenterSpec{
							ClusterName:   "Example Cluster",
							ServerVersion: "4.0.1",
						},
					},
				},
			},
			want: true,
			wantLabels: map[string]string{
				oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
				api.ClusterLabel:        "ExampleCluster",
				oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
				oplabels.NameLabel:      oplabels.NameLabelValue,
				oplabels.InstanceLabel:  fmt.Sprintf("%s-ExampleCluster", oplabels.NameLabelValue),
				oplabels.VersionLabel:   "4.0.1",
			},
		},
		{
			name: "Nil labels",
			args: args{
				resourceLabels: nil,
				rc: &ReconciliationContext{
					Datacenter: &api.CassandraDatacenter{
						ObjectMeta: metav1.ObjectMeta{
							Name: "exampleDC",
						},
						Spec: api.CassandraDatacenterSpec{
							ClusterName:   "exampleCluster",
							ServerVersion: "4.0.1",
						},
					},
				},
			},
			want: true,
			wantLabels: map[string]string{
				api.ClusterLabel:        "exampleCluster",
				oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
				oplabels.NameLabel:      oplabels.NameLabelValue,
				oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
				oplabels.InstanceLabel:  fmt.Sprintf("%s-exampleCluster", oplabels.NameLabelValue),
				oplabels.VersionLabel:   "4.0.1",
			},
		},
		{
			name: "Has Label",
			args: args{
				resourceLabels: map[string]string{
					api.ClusterLabel:        "exampleCluster",
					oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
					oplabels.NameLabel:      oplabels.NameLabelValue,
					oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
					oplabels.InstanceLabel:  fmt.Sprintf("%s-exampleCluster", oplabels.NameLabelValue),
					oplabels.VersionLabel:   "4.0.1",
				},
				rc: &ReconciliationContext{
					Datacenter: &api.CassandraDatacenter{
						ObjectMeta: metav1.ObjectMeta{
							Name: "exampleDC",
						},
						Spec: api.CassandraDatacenterSpec{
							ClusterName:   "exampleCluster",
							ServerVersion: "4.0.1",
						},
					},
				},
			},
			want: false,
			wantLabels: map[string]string{
				api.ClusterLabel:        "exampleCluster",
				oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
				oplabels.NameLabel:      oplabels.NameLabelValue,
				oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
				oplabels.InstanceLabel:  fmt.Sprintf("%s-exampleCluster", oplabels.NameLabelValue),
				oplabels.VersionLabel:   "4.0.1",
			},
		}, {
			name: "DC Label, No Cluster Label",
			args: args{
				resourceLabels: map[string]string{
					api.DatacenterLabel: "exampleDC",
				},
				rc: &ReconciliationContext{
					Datacenter: &api.CassandraDatacenter{
						ObjectMeta: metav1.ObjectMeta{
							Name: "exampleDC",
						},
						Spec: api.CassandraDatacenterSpec{
							ClusterName:   "exampleCluster",
							ServerVersion: "6.8.13",
						},
					},
				},
			},
			want: true,
			wantLabels: map[string]string{
				api.DatacenterLabel:     "exampleDC",
				api.ClusterLabel:        "exampleCluster",
				oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
				oplabels.NameLabel:      oplabels.NameLabelValue,
				oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
				oplabels.InstanceLabel:  fmt.Sprintf("%s-exampleCluster", oplabels.NameLabelValue),
				oplabels.VersionLabel:   "6.8.13",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := shouldUpdateLabelsForClusterResource(tt.args.resourceLabels, tt.args.rc.Datacenter)
			if got != tt.want {
				t.Errorf("shouldUpdateLabelsForClusterResource() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.wantLabels) {
				t.Errorf("shouldUpdateLabelsForClusterResource() got1 = %v, want %v", got1, tt.wantLabels)
			}
		})
	}
}

func TestReconcileRacks_ReconcilePods(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	var (
		one = int32(1)
	)

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	desiredStatefulSet.Spec.Replicas = &one
	desiredStatefulSet.Status.ReadyReplicas = one

	trackObjects := []runtime.Object{
		desiredStatefulSet,
		rc.Datacenter,
	}

	mockPods := mockReadyPodsForStatefulSet(desiredStatefulSet, rc.Datacenter.Spec.ClusterName, rc.Datacenter.Name)
	for idx := range mockPods {
		mp := mockPods[idx]
		trackObjects = append(trackObjects, mp)
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(rc.Datacenter).WithRuntimeObjects(trackObjects...).Build()

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 1
	nextRack.SeedCount = 1

	rackInfo := []*RackInformation{nextRack}

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.NotNil(t, result, "Result should not be nil")
}

func TestCheckRackPodTemplate_SetControllerRefOnStatefulSet(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.Racks = []api.Rack{
		{Name: "rack1"},
	}

	if err := rc.CalculateRackInformation(); err != nil {
		t.Fatalf("failed to calculate rack information: %s", err)
	}

	result := rc.CheckRackCreation()
	assert.False(t, result.Completed(), "CheckRackCreation did not complete as expected")

	if err := rc.Client.Update(rc.Ctx, rc.Datacenter); err != nil {
		t.Fatalf("failed to add rack to cassandradatacenter: %s", err)
	}

	var actualOwner, actualObject metav1.Object
	invocations := 0
	setControllerReference = func(owner, object metav1.Object, scheme *runtime.Scheme) error {
		actualOwner = owner
		actualObject = object
		invocations++
		return nil
	}

	terminationGracePeriod := int64(35)
	podTemplateSpec := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &terminationGracePeriod,
		},
	}
	rc.Datacenter.Spec.PodTemplateSpec = podTemplateSpec

	result = rc.CheckRackPodTemplate()
	assert.True(t, result.Completed())

	assert.Equal(t, 1, invocations)
	assert.Equal(t, rc.Datacenter, actualOwner)
	assert.Equal(t, rc.statefulSets[0].Name, actualObject.GetName())
}

func TestCheckRackPodTemplate_CanaryUpgrade(t *testing.T) {
	rc, _, cleanpMockSrc := setupTest()
	defer cleanpMockSrc()

	rc.Datacenter.Spec.Racks = []api.Rack{
		{Name: "rack1", DeprecatedZone: "zone-1"},
	}

	if err := rc.CalculateRackInformation(); err != nil {
		t.Fatalf("failed to calculate rack information: %s", err)
	}

	result := rc.CheckRackCreation()
	assert.False(t, result.Completed(), "CheckRackCreation did not complete as expected")

	if err := rc.Client.Update(rc.Ctx, rc.Datacenter); err != nil {
		t.Fatalf("failed to add rack to cassandradatacenter: %s", err)
	}

	result = rc.CheckRackPodTemplate()
	_, err := result.Output()

	assert.True(t, result.Completed())
	assert.Nil(t, err)

	rc.Datacenter.Spec.CanaryUpgrade = true
	rc.Datacenter.Spec.CanaryUpgradeCount = 1
	rc.Datacenter.Spec.ServerVersion = "6.8.44"
	partition := rc.Datacenter.Spec.CanaryUpgradeCount

	result = rc.CheckRackPodTemplate()
	_, err = result.Output()

	assert.True(t, result.Completed())
	assert.Nil(t, err)

	assert.Equal(t, rc.Datacenter.Status.CassandraOperatorProgress, api.ProgressUpdating)

	expectedStrategy := appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
			Partition: &partition,
		},
	}

	assert.Equal(t, expectedStrategy, rc.statefulSets[0].Spec.UpdateStrategy)

	rc.statefulSets[0].Status.Replicas = 2
	rc.statefulSets[0].Status.ReadyReplicas = 2
	rc.statefulSets[0].Status.CurrentReplicas = 1
	rc.statefulSets[0].Status.UpdatedReplicas = 1
	rc.statefulSets[0].Status.CurrentRevision = "1"
	rc.statefulSets[0].Status.UpdateRevision = "2"

	rc.Datacenter.Spec.CanaryUpgrade = false

	result = rc.CheckRackPodTemplate()
	assert.True(t, result.Completed())
	assert.NotEqual(t, expectedStrategy, rc.statefulSets[0].Spec.UpdateStrategy)
}

func TestCheckRackPodTemplate_GenerationCheck(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanpMockSrc := setupTest()
	defer cleanpMockSrc()

	require.NoError(t, rc.CalculateRackInformation())

	res := rc.CheckRackCreation()
	assert.False(res.Completed(), "CheckRackCreation did not complete as expected")

	// Update the generation manually and now verify we won't do updates to StatefulSets if the generation hasn't changed
	rc.Datacenter.Status.ObservedGeneration = rc.Datacenter.Generation
	rc.Datacenter.Spec.ServerVersion = "6.8.44"

	res = rc.CheckRackPodTemplate()
	assert.Equal(result.Continue(), res)
	cond, found := rc.Datacenter.GetCondition(api.DatacenterRequiresUpdate)
	assert.True(found)
	assert.Equal(corev1.ConditionTrue, cond.Status)

	// Verify full reconcile does not remove our updated condition
	_, err := rc.ReconcileAllRacks()
	require.NoError(t, err)
	cond, found = rc.Datacenter.GetCondition(api.DatacenterRequiresUpdate)
	assert.True(found)
	assert.Equal(corev1.ConditionTrue, cond.Status)

	// Add annotation
	metav1.SetMetaDataAnnotation(&rc.Datacenter.ObjectMeta, api.UpdateAllowedAnnotation, string(api.AllowUpdateAlways))
	rc.Datacenter.Spec.ServerVersion = "6.8.44" // This needs to be reapplied, since we call Patch in the CheckRackPodTemplate()

	res = rc.CheckRackPodTemplate()
	assert.True(res.Completed())
}

func TestCheckRackPodTemplate_RackStabilization(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.Size = 3
	rc.Datacenter.Spec.Racks = []api.Rack{
		{Name: "rack1"},
		{Name: "rack2"},
		{Name: "rack3"},
	}

	require.NoError(rc.CalculateRackInformation())

	for idx, rackInfo := range rc.desiredRackInformation {
		sts, err := newStatefulSetForCassandraDatacenter(
			nil,
			rackInfo.RackName,
			rc.Datacenter,
			1)
		require.NoError(err)

		sts.Status.Replicas = 1
		sts.Status.ReadyReplicas = 1
		sts.Status.UpdatedReplicas = 1
		sts.Status.ObservedGeneration = sts.Generation

		rc.statefulSets[idx] = sts
	}

	for idx, rackInfo := range rc.desiredRackInformation {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-0", rackInfo.RackName),
				Labels: map[string]string{
					api.RackLabel: rackInfo.RackName,
				},
			},
			Status: corev1.PodStatus{},
		}

		if idx == 0 {
			pod.Status.StartTime = &metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
		} else {
			pod.Status.StartTime = &metav1.Time{Time: time.Now().Add(-3 * time.Minute)}
		}

		rc.dcPods = append(rc.dcPods, pod)
	}

	result := rc.CheckRackPodTemplate()
	res, err := result.Output()
	require.NoError(err)
	assert.Equal(reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, res, "Should requeue due to recent pod start time in previous rack")

	// Lets make them pass this time
	rc.dcPods = []*corev1.Pod{}

	for _, rackInfo := range rc.desiredRackInformation {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-0", rackInfo.RackName),
				Labels: map[string]string{
					api.RackLabel: rackInfo.RackName,
				},
			},
			Status: corev1.PodStatus{
				StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
			},
		}
		rc.dcPods = append(rc.dcPods, pod)
	}

	setControllerReference = func(owner, object metav1.Object, scheme *runtime.Scheme) error {
		return nil
	}

	result = rc.CheckRackPodTemplate()
	assert.False(result.Completed())
}

func TestCheckRackPodTemplate_TemplateLabels(t *testing.T) {
	require := require.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.PodTemplateSpec = &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cassandra",
				},
			},
		},
	}

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	require.NoErrorf(err, "error occurred creating statefulset")

	desiredStatefulSet.Generation = 1
	desiredStatefulSet.Spec.Replicas = ptr.To(int32(1))
	desiredStatefulSet.Status.Replicas = int32(1)
	desiredStatefulSet.Status.UpdatedReplicas = int32(1)
	desiredStatefulSet.Status.ObservedGeneration = 1
	desiredStatefulSet.Status.ReadyReplicas = int32(1)

	trackObjects := []runtime.Object{
		desiredStatefulSet,
		rc.Datacenter,
	}

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 1
	nextRack.SeedCount = 1

	rackInfo := []*RackInformation{nextRack}

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))
	rc.statefulSets[0] = desiredStatefulSet

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(rc.Datacenter, rc.statefulSets[0]).WithRuntimeObjects(trackObjects...).Build()
	res := rc.CheckRackPodTemplate()
	require.Equal(result.Done(), res)
	rc.statefulSets[0].Status.ObservedGeneration = rc.statefulSets[0].Generation

	sts := &appsv1.StatefulSet{}
	require.NoError(rc.Client.Get(rc.Ctx, types.NamespacedName{Name: desiredStatefulSet.Name, Namespace: desiredStatefulSet.Namespace}, sts))
	require.Equal("bar", sts.Spec.Template.Labels["foo"])

	// Now update the template and verify that the StatefulSet is updated
	rc.Datacenter.Spec.PodTemplateSpec.ObjectMeta.Labels["foo2"] = "baz"
	rc.Datacenter.Generation++
	res = rc.CheckRackPodTemplate()
	require.Equal(result.Done(), res)

	sts = &appsv1.StatefulSet{}
	require.NoError(rc.Client.Get(rc.Ctx, types.NamespacedName{Name: desiredStatefulSet.Name, Namespace: desiredStatefulSet.Namespace}, sts))
	require.Equal("bar", sts.Spec.Template.Labels["foo"])
	require.Equal("baz", sts.Spec.Template.Labels["foo2"])
}

func TestReconcilePods(t *testing.T) {
	t.Skip()
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientGet(mockClient, nil)

	// this mock will only pass if the pod is updated with the correct labels
	mockClient.On("Update",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj *corev1.Pod) bool {
				dc := api.CassandraDatacenter{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cassandradatacenter-example",
						Namespace: "default",
					},
					Spec: api.CassandraDatacenterSpec{
						ClusterName: "cassandradatacenter-example-cluster",
					},
				}
				expected := dc.GetRackLabels("default")
				expected[oplabels.ManagedByLabel] = oplabels.ManagedByLabelValue

				return reflect.DeepEqual(obj.GetLabels(), expected)
			})).
		Return(nil).
		Once()

	statefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")
	statefulSet.Status.Replicas = int32(1)

	err = rc.ReconcilePods(statefulSet)
	assert.NoErrorf(t, err, "Should not have returned an error")

	mockClient.AssertExpectations(t)
}

func TestReconcilePods_WithVolumes(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	statefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")
	statefulSet.Status.Replicas = int32(1)

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cassandradatacenter-example-cluster-cassandradatacenter-example-default-sts-0",
			Namespace: statefulSet.Namespace,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{
				Name: "server-data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "cassandra-data-example-cluster1-example-cassandradatacenter1-rack0-sts-0",
					},
				},
			}},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName,
			Namespace: statefulSet.Namespace,
		},
	}

	trackObjects := []runtime.Object{
		pod,
		pvc,
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(pod, pvc).WithRuntimeObjects(trackObjects...).Build()
	err = rc.ReconcilePods(statefulSet)
	assert.NoErrorf(t, err, "Should not have returned an error")
}

// Note: getStatefulSetForRack is currently just a query,
// and there is really no logic to test.
// We can add a unit test later, if needed.

func TestReconcileNextRack(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	statefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	err = rc.ReconcileNextRack(statefulSet)
	assert.NoErrorf(t, err, "Should not have returned an error")

	// Validation:
	// Currently reconcileNextRack does two things
	// 1. Creates the given StatefulSet in k8s.
	// 2. Creates a PodDisruptionBudget for the StatefulSet.
	//
	// TODO: check if Create() has been called on the fake client

}

func TestReconcileNextRack_CreateError(t *testing.T) {
	t.Skip()
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	statefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientCreate(mockClient, fmt.Errorf(""))
	k8sMockClientUpdate(mockClient, nil).Times(1)

	err = rc.ReconcileNextRack(statefulSet)

	mockClient.AssertExpectations(t)

	assert.Errorf(t, err, "Should have returned an error while calculating reconciliation actions")
}

func TestCalculateRackInformation(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	err := rc.CalculateRackInformation()
	assert.NoErrorf(t, err, "Should not have returned an error")

	rackInfo := rc.desiredRackInformation[0]

	assert.Equal(t, "default", rackInfo.RackName, "Should have correct rack name")

	rc.ReqLogger.Info(
		"Node count is ",
		"Node Count: ",
		rackInfo.NodeCount)

	assert.Equal(t, 2, rackInfo.NodeCount, "Should have correct node count")

	// TODO add more RackInformation validation

}

func TestCalculateRackInformation_MultiRack(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.Racks = []api.Rack{{
		Name: "rack0",
	}, {
		Name: "rack1",
	}, {
		Name: "rack2",
	}}

	rc.Datacenter.Spec.Size = 3

	err := rc.CalculateRackInformation()
	assert.NoErrorf(t, err, "Should not have returned an error")

	rackInfo := rc.desiredRackInformation[0]

	assert.Equal(t, "rack0", rackInfo.RackName, "Should have correct rack name")

	rc.ReqLogger.Info(
		"Node count is ",
		"Node Count: ",
		rackInfo.NodeCount)

	assert.Equal(t, 1, rackInfo.NodeCount, "Should have correct node count")

	// TODO add more RackInformation validation
}

func TestReconcileRacks(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	trackObjects := []runtime.Object{
		desiredStatefulSet,
		rc.Datacenter,
	}

	mockPods := mockReadyPodsForStatefulSet(desiredStatefulSet, rc.Datacenter.Spec.ClusterName, rc.Datacenter.Name)
	for idx := range mockPods {
		mp := mockPods[idx]
		trackObjects = append(trackObjects, mp)
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(desiredStatefulSet, rc.Datacenter).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 1

	rackInfo = append(rackInfo, nextRack)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()

	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.NotNil(t, result, "Result should not be nil")
}

func TestReconcileRacks_GetStatefulsetError(t *testing.T) {
	t.Skip()
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientGet(mockClient, fmt.Errorf(""))

	var rackInfo []*RackInformation

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 1

	rackInfo = append(rackInfo, nextRack)

	rc.desiredRackInformation = rackInfo

	result, err := rc.ReconcileAllRacks()

	mockClient.AssertExpectations(t)

	assert.Errorf(t, err, "Should have returned an error")

	t.Skip("FIXME - Skipping assertion")

	assert.Equal(t, reconcile.Result{Requeue: true}, result, "Should requeue request")
}

func TestReconcileRacks_WaitingForReplicas(t *testing.T) {
	t.Skip()
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	trackObjects := []runtime.Object{
		desiredStatefulSet,
	}

	mockPods := mockReadyPodsForStatefulSet(desiredStatefulSet, rc.Datacenter.Spec.ClusterName, rc.Datacenter.Name)
	for idx := range mockPods {
		mp := mockPods[idx]
		trackObjects = append(trackObjects, mp)
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(desiredStatefulSet).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 1
	nextRack.SeedCount = 1

	rackInfo = append(rackInfo, nextRack)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.True(t, result.Requeue, result, "Should requeue request")
}

func TestReconcileRacks_NeedMoreReplicas(t *testing.T) {
	t.Skip("FIXME - Skipping test")

	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	preExistingStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	trackObjects := []runtime.Object{
		preExistingStatefulSet,
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(preExistingStatefulSet).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 3
	nextRack.SeedCount = 3

	rackInfo = append(rackInfo, nextRack)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.Equal(t, reconcile.Result{Requeue: true}, result, "Should requeue request")
}

func TestReconcileRacks_DoesntScaleDown(t *testing.T) {
	t.Skip()
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	preExistingStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	trackObjects := []runtime.Object{
		preExistingStatefulSet,
	}

	mockPods := mockReadyPodsForStatefulSet(preExistingStatefulSet, rc.Datacenter.Spec.ClusterName, rc.Datacenter.Name)
	for idx := range mockPods {
		mp := mockPods[idx]
		trackObjects = append(trackObjects, mp)
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(preExistingStatefulSet).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 1
	nextRack.SeedCount = 1

	rackInfo = append(rackInfo, nextRack)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.True(t, result.Requeue, result, "Should requeue request")
}

func TestReconcileRacks_NeedToPark(t *testing.T) {
	t.Skip()
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	preExistingStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		3)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	trackObjects := []runtime.Object{
		preExistingStatefulSet,
		rc.Datacenter,
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(preExistingStatefulSet, rc.Datacenter).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	rc.Datacenter.Spec.Stopped = true
	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 0
	nextRack.SeedCount = 0

	rackInfo = append(rackInfo, nextRack)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Apply() should not have returned an error")
	assert.False(t, result.Requeue, "Should not requeue request")

	currentStatefulSet := &appsv1.StatefulSet{}
	nsName := types.NamespacedName{Name: preExistingStatefulSet.Name, Namespace: preExistingStatefulSet.Namespace}
	err = rc.Client.Get(rc.Ctx, nsName, currentStatefulSet)
	assert.NoErrorf(t, err, "Client.Get() should not have returned an error")

	assert.Equal(t, int32(0), *currentStatefulSet.Spec.Replicas, "The statefulset should be set to zero replicas")
}

func TestReconcileRacks_AlreadyReconciled(t *testing.T) {
	t.Skip("FIXME - Skipping this test")

	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	desiredStatefulSet.Status.ReadyReplicas = 2

	desiredPdb := newPodDisruptionBudgetForDatacenter(rc.Datacenter)

	trackObjects := []runtime.Object{
		desiredStatefulSet,
		rc.Datacenter,
		desiredPdb,
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(desiredStatefulSet, rc.Datacenter, desiredPdb).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	nextRack := &RackInformation{}
	nextRack.RackName = "default"
	nextRack.NodeCount = 2
	nextRack.SeedCount = 2

	rackInfo = append(rackInfo, nextRack)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.Equal(t, reconcile.Result{}, result, "Should not requeue request")
}

func TestReconcileStatefulSet_ImmutableSpec(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	origStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"rack0",
		rc.Datacenter,
		2)
	assert.NoErrorf(err, "error occurred creating statefulset")

	assert.NotEqual("immutable-service", origStatefulSet.Spec.ServiceName)
	origStatefulSet.Spec.ServiceName = "immutable-service"

	modifiedStatefulSet, err := newStatefulSetForCassandraDatacenter(
		origStatefulSet,
		"rack0",
		rc.Datacenter,
		2)
	assert.NoErrorf(err, "error occurred creating statefulset")

	assert.Equal("immutable-service", modifiedStatefulSet.Spec.ServiceName)
}

func TestReconcileRacks_FirstRackAlreadyReconciled(t *testing.T) {
	t.Skip("FIXME - Skipping this test")

	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"rack0",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	desiredStatefulSet.Status.ReadyReplicas = 2

	secondDesiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"rack1",
		rc.Datacenter,
		1)
	assert.NoErrorf(t, err, "error occurred creating statefulset")
	secondDesiredStatefulSet.Status.ReadyReplicas = 1

	trackObjects := []runtime.Object{
		desiredStatefulSet,
		secondDesiredStatefulSet,
		rc.Datacenter,
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(desiredStatefulSet, secondDesiredStatefulSet, rc.Datacenter).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	rack0 := &RackInformation{}
	rack0.RackName = "rack0"
	rack0.NodeCount = 2
	rack0.SeedCount = 2

	rack1 := &RackInformation{}
	rack1.RackName = "rack1"
	rack1.NodeCount = 2
	rack1.SeedCount = 1

	rackInfo = append(rackInfo, rack0, rack1)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.Equal(t, reconcile.Result{Requeue: true}, result, "Should requeue request")

	currentStatefulSet := &appsv1.StatefulSet{}
	nsName := types.NamespacedName{Name: secondDesiredStatefulSet.Name, Namespace: secondDesiredStatefulSet.Namespace}
	err = rc.Client.Get(rc.Ctx, nsName, currentStatefulSet)
	assert.NoErrorf(t, err, "Client.Get() should not have returned an error")

	assert.Equal(t, int32(2), *currentStatefulSet.Spec.Replicas, "The statefulset should be set to 2 replicas")
}

func TestReconcileRacks_UpdateRackNodeCount(t *testing.T) {
	type args struct {
		rc           *ReconciliationContext
		statefulSet  *appsv1.StatefulSet
		newNodeCount int32
	}

	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	var nextRack = &RackInformation{}

	nextRack.RackName = "default"
	nextRack.NodeCount = 2

	statefulSet, _, _ := rc.GetStatefulSetForRack(nextRack)

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "check that replicas get increased",
			args: args{
				rc:           rc,
				statefulSet:  statefulSet,
				newNodeCount: 3,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trackObjects := []runtime.Object{
				tt.args.statefulSet,
				rc.Datacenter,
			}

			rc.Client = fake.NewClientBuilder().WithStatusSubresource(tt.args.statefulSet, rc.Datacenter).WithRuntimeObjects(trackObjects...).Build()

			if err := rc.UpdateRackNodeCount(tt.args.statefulSet, tt.args.newNodeCount); (err != nil) != tt.wantErr {
				t.Errorf("updateRackNodeCount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.args.newNodeCount != *tt.args.statefulSet.Spec.Replicas {
				t.Errorf("StatefulSet spec should have different replica count, has = %v, want %v", *tt.args.statefulSet.Spec.Replicas, tt.args.newNodeCount)
			}
		})
	}
}

func TestReconcileRacks_UpdateConfig(t *testing.T) {
	t.Skip("FIXME - Skipping this test")

	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"rack0",
		rc.Datacenter,
		2)
	assert.NoErrorf(t, err, "error occurred creating statefulset")

	desiredStatefulSet.Status.ReadyReplicas = 2

	desiredPdb := newPodDisruptionBudgetForDatacenter(rc.Datacenter)

	mockPods := mockReadyPodsForStatefulSet(desiredStatefulSet, rc.Datacenter.Spec.ClusterName, rc.Datacenter.Name)

	trackObjects := []runtime.Object{
		desiredStatefulSet,
		rc.Datacenter,
		desiredPdb,
	}
	for idx := range mockPods {
		mp := mockPods[idx]
		trackObjects = append(trackObjects, mp)
	}

	rc.Client = fake.NewClientBuilder().WithStatusSubresource(desiredStatefulSet, rc.Datacenter, desiredPdb).WithRuntimeObjects(trackObjects...).Build()

	var rackInfo []*RackInformation

	rack0 := &RackInformation{}
	rack0.RackName = "rack0"
	rack0.NodeCount = 2

	rackInfo = append(rackInfo, rack0)

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err := rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.Equal(t, reconcile.Result{Requeue: false}, result, "Should not requeue request")

	currentStatefulSet := &appsv1.StatefulSet{}
	nsName := types.NamespacedName{Name: desiredStatefulSet.Name, Namespace: desiredStatefulSet.Namespace}
	err = rc.Client.Get(rc.Ctx, nsName, currentStatefulSet)
	assert.NoErrorf(t, err, "Client.Get() should not have returned an error")

	assert.Equal(t,
		"{\"cluster-info\":{\"name\":\"cassandradatacenter-example-cluster\",\"seeds\":\"cassandradatacenter-example-cluster-seed-service\"},\"datacenter-info\":{\"name\":\"cassandradatacenter-example\"}}",
		currentStatefulSet.Spec.Template.Spec.InitContainers[0].Env[0].Value,
		"The statefulset env config should not contain a cassandra-yaml entry.")

	// Update the config and rerun the reconcile

	configJson := []byte("{\"cassandra-yaml\":{\"authenticator\":\"AllowAllAuthenticator\"}}")

	rc.Datacenter.Spec.Config = configJson

	rc.desiredRackInformation = rackInfo
	rc.statefulSets = make([]*appsv1.StatefulSet, len(rackInfo))

	result, err = rc.ReconcileAllRacks()
	assert.NoErrorf(t, err, "Should not have returned an error")
	assert.Equal(t, reconcile.Result{Requeue: true}, result, "Should requeue request")

	currentStatefulSet = &appsv1.StatefulSet{}
	nsName = types.NamespacedName{Name: desiredStatefulSet.Name, Namespace: desiredStatefulSet.Namespace}
	err = rc.Client.Get(rc.Ctx, nsName, currentStatefulSet)
	assert.NoErrorf(t, err, "Client.Get() should not have returned an error")

	assert.Equal(t,
		"{\"cassandra-yaml\":{\"authenticator\":\"AllowAllAuthenticator\"},\"cluster-info\":{\"name\":\"cassandradatacenter-example-cluster\",\"seeds\":\"cassandradatacenter-example-cluster-seed-service\"},\"datacenter-info\":{\"name\":\"cassandradatacenter-example\"}}",
		currentStatefulSet.Spec.Template.Spec.InitContainers[0].Env[0].Value,
		"The statefulset should contain a cassandra-yaml entry.")
}

func mockReadyPodsForStatefulSet(sts *appsv1.StatefulSet, cluster, dc string) []*corev1.Pod {
	var pods []*corev1.Pod
	sz := int(*sts.Spec.Replicas)
	for i := 0; i < sz; i++ {
		pod := &corev1.Pod{}
		pod.Namespace = sts.Namespace
		pod.Name = fmt.Sprintf("%s-%d", sts.Name, i)
		pod.Labels = make(map[string]string)
		pod.Labels[api.ClusterLabel] = cluster
		pod.Labels[api.DatacenterLabel] = dc
		pod.Labels[api.CassNodeState] = "Started"
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Ready: true,
		}}
		pods = append(pods, pod)
	}
	return pods
}

func makeMockReadyStartedPod() *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Labels = make(map[string]string)
	pod.Labels[api.CassNodeState] = "Started"
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  "cassandra",
		Ready: true,
	}}
	return pod
}

func TestReconcileRacks_countReadyAndStarted(t *testing.T) {
	type fields struct {
		ReconcileContext       *ReconciliationContext
		desiredRackInformation []*RackInformation
		statefulSets           []*appsv1.StatefulSet
	}
	type args struct {
		podList *corev1.PodList
	}
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantReady   int
		wantStarted int
	}{
		{
			name: "test an empty podList",
			fields: fields{
				ReconcileContext:       rc,
				desiredRackInformation: []*RackInformation{},
				statefulSets:           []*appsv1.StatefulSet{},
			},
			args: args{
				podList: &corev1.PodList{},
			},
			wantReady:   0,
			wantStarted: 0,
		},
		{
			name: "test two ready and started pods",
			fields: fields{
				ReconcileContext:       rc,
				desiredRackInformation: []*RackInformation{},
				statefulSets:           []*appsv1.StatefulSet{},
			},
			args: args{
				podList: &corev1.PodList{
					Items: []corev1.Pod{
						*makeMockReadyStartedPod(),
						*makeMockReadyStartedPod(),
					},
				},
			},
			wantReady:   2,
			wantStarted: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc.desiredRackInformation = tt.fields.desiredRackInformation
			rc.statefulSets = tt.fields.statefulSets
			rc.dcPods = PodPtrsFromPodList(tt.args.podList)

			ready, started := rc.countReadyAndStarted()
			if ready != tt.wantReady {
				t.Errorf("ReconcileRacks.countReadyAndStarted() ready = %v, want %v", ready, tt.wantReady)
			}
			if started != tt.wantStarted {
				t.Errorf("ReconcileRacks.countReadyAndStarted() started = %v, want %v", started, tt.wantStarted)
			}
		})
	}
}

func Test_isServerReady(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	podThatHasNoServer := makeMockReadyStartedPod()
	podThatHasNoServer.Status.ContainerStatuses[0].Name = "nginx"
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "check a ready server pod",
			args: args{
				pod: makeMockReadyStartedPod(),
			},
			want: true,
		},
		{
			name: "check a ready non-server pod",
			args: args{
				pod: podThatHasNoServer,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isServerReady(tt.args.pod); got != tt.want {
				t.Errorf("isServerReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isMgmtApiRunning(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	readyServerContainer := makeMockReadyStartedPod()
	readyServerContainer.Status.ContainerStatuses[0].State.Running =
		&corev1.ContainerStateRunning{StartedAt: metav1.Date(2019, time.July, 4, 12, 12, 12, 0, time.UTC)}

	veryFreshServerContainer := makeMockReadyStartedPod()
	veryFreshServerContainer.Status.ContainerStatuses[0].State.Running =
		&corev1.ContainerStateRunning{StartedAt: metav1.Now()}

	podThatHasNoServer := makeMockReadyStartedPod()
	podThatHasNoServer.Status.ContainerStatuses[0].Name = "nginx"

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "check a ready server pod",
			args: args{
				pod: readyServerContainer,
			},
			want: true,
		},
		{
			name: "check a ready server pod that started as recently as possible",
			args: args{
				pod: veryFreshServerContainer,
			},
			want: false,
		},
		{
			name: "check a ready server pod that started as recently as possible",
			args: args{
				pod: podThatHasNoServer,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMgmtApiRunning(tt.args.pod); got != tt.want {
				t.Errorf("isMgmtApiRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_shouldUpdateLabelsForRackResource(t *testing.T) {
	clusterName := "cassandradatacenter-example-cluster"
	dcName := "cassandradatacenter-example"
	rackName := "rack1"
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dcName,
			Namespace: "default",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   clusterName,
			ServerVersion: "4.0.1",
		},
	}

	goodRackLabels := map[string]string{
		api.ClusterLabel:        clusterName,
		api.DatacenterLabel:     dcName,
		api.RackLabel:           rackName,
		oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
		oplabels.NameLabel:      oplabels.NameLabelValue,
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, clusterName),
		oplabels.VersionLabel:   "4.0.1",
	}

	type args struct {
		resourceLabels map[string]string
	}

	type result struct {
		changed bool
		labels  map[string]string
	}

	// cases where label updates are made
	tests := []struct {
		name string
		args args
		want result
	}{
		{
			name: "Cluster name different",
			args: args{
				resourceLabels: map[string]string{
					api.ClusterLabel:    "some-other-cluster",
					api.DatacenterLabel: dcName,
					api.RackLabel:       rackName,
				},
			},
			want: result{
				changed: true,
				labels:  goodRackLabels,
			},
		},
		{
			name: "Rack name different",
			args: args{
				resourceLabels: map[string]string{
					api.ClusterLabel:    clusterName,
					api.DatacenterLabel: dcName,
					api.RackLabel:       "some-other-rack",
				},
			},
			want: result{
				changed: true,
				labels:  goodRackLabels,
			},
		},
		{
			name: "Rack name different plus other labels",
			args: args{
				resourceLabels: map[string]string{
					api.ClusterLabel:    clusterName,
					api.DatacenterLabel: dcName,
					api.RackLabel:       "some-other-rack",
					"foo":               "bar",
				},
			},
			want: result{
				changed: true,
				labels: utils.MergeMap(
					map[string]string{},
					goodRackLabels,
					map[string]string{"foo": "bar"}),
			},
		},
		{
			name: "No labels",
			args: args{
				resourceLabels: map[string]string{},
			},
			want: result{
				changed: true,
				labels:  goodRackLabels,
			},
		},
		{
			name: "Correct labels",
			args: args{
				resourceLabels: map[string]string{
					api.ClusterLabel:        clusterName,
					api.DatacenterLabel:     dcName,
					api.RackLabel:           rackName,
					oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
					oplabels.NameLabel:      oplabels.NameLabelValue,
					oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
					oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, clusterName),
					oplabels.VersionLabel:   "4.0.1",
				},
			},
			want: result{
				changed: false,
			},
		},
		{
			name: "Correct labels with some additional labels",
			args: args{
				resourceLabels: map[string]string{
					api.ClusterLabel:        clusterName,
					api.DatacenterLabel:     dcName,
					api.RackLabel:           rackName,
					oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
					oplabels.NameLabel:      oplabels.NameLabelValue,
					oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
					oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, clusterName),
					oplabels.VersionLabel:   "4.0.1",
					"foo":                   "bar",
				},
			},
			want: result{
				changed: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want.changed {
				changed, newLabels := shouldUpdateLabelsForRackResource(tt.args.resourceLabels, dc, rackName)
				if !changed || !reflect.DeepEqual(newLabels, tt.want.labels) {
					t.Errorf("shouldUpdateLabelsForRackResource() = (%v, %v), want (%v, %v)", changed, newLabels, true, tt.want)
				}
			} else {
				// when the labels aren't supposed to be changed, we want to
				// make sure that the map returned *is* the map passed in and
				// that it is unchanged.
				resourceLabelsCopy := utils.MergeMap(map[string]string{}, tt.args.resourceLabels)
				changed, newLabels := shouldUpdateLabelsForRackResource(tt.args.resourceLabels, dc, rackName)
				if changed || !reflect.DeepEqual(resourceLabelsCopy, newLabels) {
					t.Errorf("shouldUpdateLabelsForRackResource() = (%v, %v), want (%v, %v)", changed, newLabels, true, tt.want)
				} else if reflect.ValueOf(tt.args.resourceLabels).Pointer() != reflect.ValueOf(newLabels).Pointer() {
					t.Error("shouldUpdateLabelsForRackResource() did not return original map")
				}
			}
		})
	}
}

func makeReloadTestPod() *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "default",
			Labels: map[string]string{
				api.ClusterLabel:    "mycluster",
				api.DatacenterLabel: "mydc",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "127.0.0.1",
		},
	}
	return pod
}

func Test_callPodEndpoint(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("OK")),
	}

	mockHttpClient := mocks.NewHttpClient(t)
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req != nil
			})).
		Return(res, nil).
		Once()

	client := httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	pod := makeReloadTestPod()
	pod.Status.PodIP = "1.2.3.4"

	if err := client.CallReloadSeedsEndpoint(pod); err != nil {
		assert.Fail(t, "Should not have returned error")
	}
}

func Test_callPodEndpoint_BadStatus(t *testing.T) {
	res := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader("OK")),
	}

	mockHttpClient := mocks.NewHttpClient(t)
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/ops/seeds/reload" && req.Method == "POST"
			})).
		Return(res, nil).
		Once()

	client := httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      zap.New(),
		Protocol: "http",
	}

	pod := makeReloadTestPod()

	if err := client.CallReloadSeedsEndpoint(pod); err == nil {
		assert.Fail(t, "Should have returned error")
	}
}

func Test_callPodEndpoint_RequestFail(t *testing.T) {
	res := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader("OK")),
	}

	mockHttpClient := mocks.NewHttpClient(t)
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req != nil
			})).
		Return(res, fmt.Errorf("")).
		Once()

	client := httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      zap.New(),
		Protocol: "http",
	}

	pod := makeReloadTestPod()

	if err := client.CallReloadSeedsEndpoint(pod); err == nil {
		assert.Fail(t, "Should have returned error")
	}
}

func TestCleanupAfterScaling(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	assert := assert.New(t)

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	var task *taskapi.CassandraTask
	// 1. Create task - return ok
	k8sMockClientCreate(rc.Client.(*mocks.Client), nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*taskapi.CassandraTask)
			task = arg
		}).
		Times(1)

	r := rc.cleanupAfterScaling()
	assert.Equal(result.Continue(), r, "expected result of result.Continue()")
	assert.Equal(taskapi.CommandCleanup, task.Spec.Jobs[0].Command)
}

func TestStripPassword(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	password := "secretPassword"

	mockHttpClient := mocks.NewHttpClient(t)
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req != nil
			})).
		Return(nil, fmt.Errorf(password)).
		Once()

	client := httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	pod := makeReloadTestPod()
	pod.Status.PodIP = "1.2.3.4"

	err := client.CallCreateRoleEndpoint(pod, "userNameA", password, true)
	if err == nil {
		assert.Fail(t, "Should have returned error")
	}

	assert.False(t, strings.Contains(err.Error(), password))
}

func TestNodereplacements(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1-default-sts-0",
		},
	}

	pod2 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1-default-sts-1",
		},
	}

	rc.dcPods = []*corev1.Pod{
		pod, pod2,
	}

	err := rc.startReplacePodsIfReplacePodsSpecified()
	assert.NoError(err)
	assert.Equal(0, len(rc.Datacenter.Status.NodeReplacements))

	rc.Datacenter.Spec.DeprecatedReplaceNodes = []string{""}
	err = rc.startReplacePodsIfReplacePodsSpecified()
	assert.NoError(err)
	assert.Equal(0, len(rc.Datacenter.Status.NodeReplacements))
	assert.Equal(0, len(rc.Datacenter.Spec.DeprecatedReplaceNodes))

	rc.Datacenter.Spec.DeprecatedReplaceNodes = []string{"dc1-default-sts-3"} // Does not exist
	err = rc.startReplacePodsIfReplacePodsSpecified()
	assert.NoError(err)
	assert.Equal(0, len(rc.Datacenter.Status.NodeReplacements))
	assert.Equal(0, len(rc.Datacenter.Spec.DeprecatedReplaceNodes))

	rc.Datacenter.Spec.DeprecatedReplaceNodes = []string{"dc1-default-sts-0"}
	err = rc.startReplacePodsIfReplacePodsSpecified()
	assert.NoError(err)
	assert.Equal(1, len(rc.Datacenter.Status.NodeReplacements))
	assert.Equal(0, len(rc.Datacenter.Spec.DeprecatedReplaceNodes))
}

// TestFailedStart verifies the pod is deleted if nodeMgmtClient start fails
func TestFailedStart(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	done := make(chan struct{})
	k8sMockClientDelete(mockClient, nil).Once().Run(func(mock.Arguments) { close(done) })

	// Patch labelStarting, lastNodeStarted..
	k8sMockClientPatch(mockClient, nil).Once()
	k8sMockClientStatusPatch(mockClient.Status().(*mocks.SubResourceClient), nil).Once()

	res := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader("OK")),
	}

	mockHttpClient := mocks.NewHttpClient(t)
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req != nil
			})).
		Return(res, nil).
		Once()

	client := httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	rc.NodeMgmtClient = client

	epData := httphelper.CassMetadataEndpoints{
		Entity: []httphelper.EndpointState{},
	}

	pod := makeReloadTestPod()

	fakeRecorder := record.NewFakeRecorder(5)
	rc.Recorder = fakeRecorder

	err := rc.startCassandra(epData, pod)
	// The start is async method, so the error is not returned here
	assert.Nil(t, err)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "No pod delete occurred")
	}

	// mockClient.AssertExpectations(t)
	// mockHttpClient.AssertExpectations(t)

	close(fakeRecorder.Events)
	// Should have 2 events, one to indicate Cassandra is starting, one to indicate it failed to start
	assert.Equal(t, 2, len(fakeRecorder.Events))
}

func TestReconciliationContext_startAllNodes(t *testing.T) {

	// A boolean representing the state of a pod (started or not).
	type pod bool

	// racks is a map of rack names to a list of pods in that rack.
	type racks map[string][]pod

	tests := []struct {
		name         string
		racks        racks
		wantNotReady bool
		wantEvents   []string
	}{
		{
			name: "balanced racks, all started",
			racks: racks{
				"rack1": {true, true, true},
				"rack2": {true, true, true},
				"rack3": {true, true, true},
			},
			wantNotReady: false,
		},
		{
			name: "balanced racks, some pods not started",
			racks: racks{
				"rack1": {true, true, false},
				"rack2": {true, false, false},
				"rack3": {true, false, false},
			},
			wantNotReady: true,
			wantEvents:   []string{"Normal StartingCassandra Starting Cassandra for pod rack2-1"},
		},
		{
			name: "unbalanced racks, all started",
			racks: racks{
				"rack1": {true, true},
				"rack2": {true},
				"rack3": {true, true, true},
			},
			wantNotReady: false,
		},
		{
			name: "unbalanced racks, some pods not started",
			racks: racks{
				"rack1": {true, true},
				"rack2": {true},
				"rack3": {true, true, false},
			},
			wantNotReady: true,
			wantEvents:   []string{"Normal StartingCassandra Starting Cassandra for pod rack3-2"},
		},
		{
			name: "unbalanced racks, part of decommission",
			racks: racks{
				"rack1": {},
				"rack2": {true},
				"rack3": {true},
			},
			wantNotReady: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, _, _ := setupTest()
			for _, rackName := range []string{"rack1", "rack2", "rack3"} {
				rackPods := tt.racks[rackName]
				sts := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{Name: rackName},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To(int32(len(rackPods)))},
				}
				rc.statefulSets = append(rc.statefulSets, sts)
				for i, started := range rackPods {
					p := &corev1.Pod{}
					p.Name = getStatefulSetPodNameForIdx(sts, int32(i))
					p.Labels = map[string]string{}
					p.Status.ContainerStatuses = []corev1.ContainerStatus{
						{
							Name: "cassandra",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metav1.Time{Time: time.Now().Add(-time.Minute)},
								},
							},
							Ready: bool(started),
						},
					}
					p.Status.PodIP = "127.0.0.1"
					if started {
						p.Labels[api.CassNodeState] = stateStarted
					} else {
						p.Labels[api.CassNodeState] = stateReadyToStart
					}
					rc.dcPods = append(rc.dcPods, p)
				}
			}

			mockClient := mocks.NewClient(t)
			rc.Client = mockClient

			done := make(chan struct{})
			if tt.wantNotReady {
				// mock the calls in labelServerPodStarting:
				// patch the pod: pod.Labels[api.CassNodeState] = stateStarting
				k8sMockClientPatch(mockClient, nil)
				// patch the dc status: dc.Status.LastServerNodeStarted = metav1.Now()
				k8sMockClientStatusPatch(mockClient.Status().(*mocks.SubResourceClient), nil)

				res := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("OK")),
				}

				mockHttpClient := mocks.NewHttpClient(t)
				mockHttpClient.On("Do",
					mock.MatchedBy(
						func(req *http.Request) bool {
							return req != nil
						})).
					Return(res, nil).
					Once().
					Run(func(mock.Arguments) { close(done) })

				client := httphelper.NodeMgmtClient{
					Client:   mockHttpClient,
					Log:      rc.ReqLogger,
					Protocol: "http",
				}
				rc.NodeMgmtClient = client

			}

			epData := httphelper.CassMetadataEndpoints{
				Entity: []httphelper.EndpointState{},
			}

			gotNotReady, err := rc.startAllNodes(epData)

			assert.NoError(t, err)
			assert.Equalf(t, tt.wantNotReady, gotNotReady, "expected not ready to be %v", tt.wantNotReady)

			if tt.wantNotReady {
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					assert.Fail(t, "No pod start occurred")
				}
			}

			fakeRecorder := rc.Recorder.(*record.FakeRecorder)
			close(fakeRecorder.Events)
			if assert.Lenf(t, fakeRecorder.Events, len(tt.wantEvents), "expected %d events, got %d", len(tt.wantEvents), len(fakeRecorder.Events)) {
				var gotEvents []string
				for i := range fakeRecorder.Events {
					gotEvents = append(gotEvents, i)
				}
				assert.Equal(t, tt.wantEvents, gotEvents)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestStartOneNodePerRack(t *testing.T) {
	// A boolean representing the state of a pod (started or not).
	type pod bool

	// racks is a map of rack names to a list of pods in that rack.
	type racks map[string][]pod

	tests := []struct {
		name         string
		racks        racks
		wantNotReady bool
		seedCount    int
	}{
		{
			name: "balanced racks, all nodes started",
			racks: racks{
				"rack1": {true, true, true},
				"rack2": {true, true, true},
				"rack3": {true, true, true},
			},
			wantNotReady: false,
			seedCount:    3,
		},
		{
			name: "balanced racks, missing nodes",
			racks: racks{
				"rack1": {true},
				"rack2": {true},
				"rack3": {false},
			},
			wantNotReady: true,
			seedCount:    2,
		},
		{
			name: "balanced racks, nothing started",
			racks: racks{
				"rack1": {false, false, false},
				"rack2": {false, false, false},
				"rack3": {false, false, false},
			},
			wantNotReady: true,
			seedCount:    0,
		},
		{
			name: "unbalanced racks, part of decommission",
			racks: racks{
				"rack1": {},
				"rack2": {true},
				"rack3": {true},
			},
			wantNotReady: false,
			seedCount:    2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, _, _ := setupTest()
			for rackName, rackPods := range tt.racks {
				sts := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{Name: rackName},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To(int32(len(rackPods)))},
				}
				rc.statefulSets = append(rc.statefulSets, sts)
				for i, started := range rackPods {
					p := &corev1.Pod{}
					p.Name = getStatefulSetPodNameForIdx(sts, int32(i))
					p.Labels = map[string]string{}
					p.Status.ContainerStatuses = []corev1.ContainerStatus{
						{
							Name: "cassandra",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metav1.Time{Time: time.Now().Add(-time.Minute)},
								},
							},
							Ready: bool(started),
						},
					}
					p.Status.PodIP = "127.0.0.1"
					if started {
						p.Labels[api.CassNodeState] = stateStarted
					} else {
						p.Labels[api.CassNodeState] = stateReadyToStart
					}
					rc.dcPods = append(rc.dcPods, p)
				}
			}

			mockClient := mocks.NewClient(t)
			rc.Client = mockClient

			done := make(chan struct{})

			if tt.wantNotReady {
				res := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("OK")),
				}

				mockHttpClient := mocks.NewHttpClient(t)
				mockHttpClient.On("Do",
					mock.MatchedBy(
						func(req *http.Request) bool {
							return req != nil
						})).
					Return(res, nil).
					Once().
					Run(func(args mock.Arguments) { close(done) })

				client := httphelper.NodeMgmtClient{
					Client:   mockHttpClient,
					Log:      rc.ReqLogger,
					Protocol: "http",
				}
				rc.NodeMgmtClient = client

				// mock the calls in labelServerPodStarting:
				// patch the pod: pod.Labels[api.CassNodeState] = stateStarting
				k8sMockClientPatch(mockClient, nil)
				// get the status client
				// patch the dc status: dc.Status.LastServerNodeStarted = metav1.Now()
				k8sMockClientStatusPatch(mockClient.Status().(*mocks.SubResourceClient), nil)

				if tt.seedCount < 1 {
					// There's additional checks here, for fetching the possible additional-seeds (the GET) and pre-adding a seed label
					k8sMockClientGet(mockClient, nil)
					k8sMockClientPatch(mockClient, nil)
				}
			}

			epData := httphelper.CassMetadataEndpoints{
				Entity: []httphelper.EndpointState{},
			}

			gotNotReady, err := rc.startOneNodePerRack(epData, tt.seedCount)

			if tt.wantNotReady {
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					assert.Fail(t, "No pod start occurred")
				}
			}

			assert.NoError(t, err)
			assert.Equalf(t, tt.wantNotReady, gotNotReady, "expected not ready to be %v", tt.wantNotReady)
		})
	}
}

func TestFindHostIdForIpFromEndpointsData(t *testing.T) {
	endpoints := []httphelper.EndpointState{
		{
			HostID:     "1",
			RpcAddress: "127.0.0.1",
		},
		{
			HostID:     "2",
			RpcAddress: "::1",
		},
		{
			HostID:     "3",
			RpcAddress: "2001:0DB8:0:0:8:800:200C:417A",
		},
	}

	assert.Equal(t, "1", findHostIdForIpFromEndpointsData(endpoints, "127.0.0.1"))
	assert.Equal(t, "2", findHostIdForIpFromEndpointsData(endpoints, "0:0:0:0:0:0:0:1"))
	assert.Equal(t, "3", findHostIdForIpFromEndpointsData(endpoints, "2001:0DB8::8:800:200C:417A"))
	assert.Equal(t, "", findHostIdForIpFromEndpointsData(endpoints, "192.168.1.0"))
}
