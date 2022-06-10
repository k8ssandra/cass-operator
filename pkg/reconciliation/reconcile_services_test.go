// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/k8ssandra/cass-operator/pkg/utils"
)

func TestReconcileHeadlessService(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	recResult := rc.CheckHeadlessServices()

	// kind of weird to check this path we don't want in a test, but
	// it's useful to see what the error is
	if recResult.Completed() {
		_, err := recResult.Output()
		assert.NoErrorf(t, err, "Should not have returned an error")
	}

	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")
}

func TestReconcileHeadlessService_UpdateLabelsAndAnnotations(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := &mocks.Client{}
	rc.Client = mockClient

	// place holder for service label maps
	svcLabelMap := make(map[string]map[string]string)
	// place holder for service annotation maps
	svcAnnotationMap :=  make(map[string]map[string]string)

	k8sMockClientGet(mockClient, nil).
		Times(4)
	k8sMockClientUpdate(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.Service)
			// store service labels
			svcLabelMap[arg.GetName()] = arg.GetLabels()
			// store svc annotations
			svcAnnotationMap[arg.GetName()] = arg.GetAnnotations()
		}).
		Times(4)

	// Check the service should populate labels and annotations
	recResult := rc.CheckHeadlessServices()

	// kind of weird to check this path we don't want in a test, but
	// it's useful to see what the error is
	if recResult.Completed() {
		_, err := recResult.Output()
		assert.NoErrorf(t, err, "Should not have returned an error")
	}

	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")

	// Mock the Datacenter Service to have additional labels
	dcSvcName := rc.Datacenter.GetDatacenterServiceName()
	assert.Containsf(t, svcLabelMap, dcSvcName, "Expected Datacenter service to be in service map. Expected name: %s, service map:\n%v\n", dcSvcName, svcLabelMap)
	dcSvcLabels := svcLabelMap[dcSvcName]
	dcSvcLabels["AddKey1"] = "Value1"
	dcSvcLabels["AddKey2"] = "Value2"
	dcSvcAnnotations := svcAnnotationMap[dcSvcName]
	dcSvcAnnotations["AddAnnotation1"] = "AddValue1"
	dcSvcAnnotations["AddAnnotation2"] = "AddValue2"
	// In DC Additional Labels, add a label, change a label value and delete a label
	rc.Datacenter.Spec.AdditionalServiceConfig.DatacenterService.Labels = map[string]string{"AddKey1" : "ChangeValue1", "AddKey3" : "Value3"}
	updatedDcSvcLabels := make(map[string]string)
	// copy current labels into updated labels
	for k, v := range dcSvcLabels {
		updatedDcSvcLabels[k] = v
	}
	delete(updatedDcSvcLabels, "AddKey2")
	updatedDcSvcLabels["AddKey1"] = "ChangeValue1"
	updatedDcSvcLabels["AddKey3"] = "Value3"
	// In DC Additional Annotations, add an annotation, change an annotation value and delete an annotation
	rc.Datacenter.Spec.AdditionalServiceConfig.DatacenterService.Annotations = map[string]string{"AddAnnotation1": "ChangeAnnotation1", "AddAnnotation3" : "AddValue3"}
	updatedDcSvcAnnotations := make(map[string]string)
	// copy current annotations into updated annotations
	for k, v := range dcSvcAnnotations {
		updatedDcSvcAnnotations[k] = v
	}
	delete(updatedDcSvcAnnotations, "AddAnnotation2")
	updatedDcSvcAnnotations["AddAnnotation1"] = "ChangeAnnotation1"
	updatedDcSvcAnnotations["AddAnnotation3"] = "AddValue3"
	// resource hash annotation will change, so exclude it from the comparison
	delete(updatedDcSvcAnnotations, utils.ResourceHashAnnotationKey)

	k8sMockClientGet(mockClient, nil).
		Run(func(args mock.Arguments) {
			svcName := args.Get(1).(types.NamespacedName)
			arg := args.Get(2).(*corev1.Service)
			if (svcName.Name == dcSvcName) {
				// set the expected service labels
				arg.SetLabels(dcSvcLabels)
				// set the expected service annotations
				arg.SetAnnotations(dcSvcAnnotations)
			}
		}).
		Times(4)
	k8sMockClientUpdate(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.Service)
			// store service labels
			svcLabelMap[arg.GetName()] = arg.GetLabels()
			// store service annotations
			svcAnnotationMap[arg.GetName()] = arg.GetAnnotations()
			// verify additional labels and annotations are added for the Datacenter Service
			if (arg.GetName() == dcSvcName) {
			    assert.Truef(t, reflect.DeepEqual(arg.GetLabels(), updatedDcSvcLabels), "Datacenter Service Labels do not match. Expected:\n%v\nObserved:\n%v\n", updatedDcSvcLabels, arg.GetLabels())
				// resource hash annotation will change, so exclude it from the comparison
				observedAnnotations := arg.GetAnnotations()
				delete(observedAnnotations, utils.ResourceHashAnnotationKey)
				assert.Truef(t, reflect.DeepEqual(arg.GetAnnotations(), updatedDcSvcAnnotations), "Datacenter Service Annotations do not match. Expected:\n%v\nObserved:\n%v\n", updatedDcSvcAnnotations, observedAnnotations)
			}
		}).
		Times(4)

	// re-populate labels and annotations
	recResult = rc.CheckHeadlessServices()


	// kind of weird to check this path we don't want in a test, but
	// it's useful to see what the error is
	if recResult.Completed() {
		_, err := recResult.Output()
		assert.NoErrorf(t, err, "Should not have returned an error")
	}

	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")
}

func TestCreateHeadlessService(t *testing.T) {
	rc, svc, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Services = []*corev1.Service{svc}

	recResult := rc.CreateHeadlessServices()

	// kind of weird to check this path we don't want in a test, but
	// it's useful to see what the error is
	if recResult.Completed() {
		_, err := recResult.Output()
		assert.NoErrorf(t, err, "Should not have returned an error")
	}

	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")
}

func TestCreateHeadlessService_ClientReturnsError(t *testing.T) {
	// skipped because mocking Status() call and response is very tricky
	t.Skip()
	rc, svc, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := &mocks.Client{}
	rc.Client = mockClient

	k8sMockClientCreate(mockClient, fmt.Errorf(""))
	k8sMockClientUpdate(mockClient, nil).Times(1)

	rc.Services = []*corev1.Service{svc}

	recResult := rc.CreateHeadlessServices()

	// kind of weird to check this path we don't want in a test, but
	// it's useful to see what the error is
	if recResult.Completed() {
		_, err := recResult.Output()
		assert.NoErrorf(t, err, "Should not have returned an error")
	}

	assert.True(t, recResult.Completed(), "Reconcile loop should be completed")

	mockClient.AssertExpectations(t)
}
