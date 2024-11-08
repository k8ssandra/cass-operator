package scheduler

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResources(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()
	cli := createClient()
	require.NoError(cli.Create(ctx, makeNode("node1")))

	pods := []*corev1.Pod{
		makePod("pod1"),
	}
	require.NoError(WillTheyFit(ctx, cli, pods))

	// Lets add more pods than the node can handle
	for i := 2; i < 20; i++ {
		pods = append(pods, makePod("pod"+strconv.Itoa(i)))
	}
	require.Error(WillTheyFit(ctx, cli, pods))
}

func createClient() client.Client {
	s := runtime.NewScheme()
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Node{})

	nodeNameIndexer := client.IndexerFunc(func(obj client.Object) []string {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}
		return []string{pod.Spec.NodeName}
	})

	fakeClient := fake.NewClientBuilder().WithIndex(&corev1.Pod{}, "spec.nodeName", nodeNameIndexer).Build()
	return fakeClient
}

func makeResources(milliCPU, memory, pods int64) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI),
		corev1.ResourcePods:   *resource.NewQuantity(pods, resource.DecimalSI),
	}
}

func makeNode(name string) *corev1.Node {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
	}
	n.Status.Capacity, n.Status.Allocatable = makeResources(1000, 1000, 100), makeResources(1000, 1000, 100)
	return n
}

func makePod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: makeResources(100, 100, 1),
						Limits:   makeResources(200, 200, 1),
					},
				},
			},
		},
	}
}
