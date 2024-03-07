package envtest

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// StatefulSetReconciler is intended to be used with envtests. It mimics some parts of the StS controller to hide such details from the tests
type StatefulSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Kubebuilder rights intentionally omitted - this only works in the envtest without rights restrictions
func (r *StatefulSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Processing StatefulSet", "NamespacedName", req.NamespacedName)

	var sts appsv1.StatefulSet
	if err := r.Get(ctx, req.NamespacedName, &sts); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	intendedReplicas := int(*sts.Spec.Replicas)

	if sts.GetDeletionTimestamp() != nil {
		logger.Info("StatefulSet has been marked for deletion")
		// Delete the pods
		for i := 0; i < intendedReplicas; i++ {

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%d", sts.Name, i),
					Namespace: sts.Namespace,
				},
			}

			if err := r.Client.Delete(ctx, pod); err != nil {
				logger.Error(err, "Failed to delete the pod")
				return ctrl.Result{}, err
			}
		}

		if controllerutil.RemoveFinalizer(&sts, "test.k8ssandra.io/sts-finalizer") {
			if err := r.Client.Update(ctx, &sts); err != nil {
				logger.Error(err, "Failed to remove finalizer from StatefulSet")
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(&sts, "test.k8ssandra.io/sts-finalizer") {
		if err := r.Client.Update(ctx, &sts); err != nil {
			logger.Error(err, "Failed to set finalizer to StatefulSet")
			return ctrl.Result{}, err
		}
	}

	// TODO Implement new parts as needed for tests
	// TODO Get existing pods and modify them .

	podList := &corev1.PodList{}
	if err := r.Client.List(ctx, podList, client.MatchingLabels(sts.Spec.Template.Labels), client.InNamespace(req.Namespace)); err != nil {
		logger.Error(err, "Failed to list the pods belonging to this StatefulSet")
		return ctrl.Result{}, err
	}

	stsPods := []*corev1.Pod{}
	for _, pod := range podList.Items {
		if pod.OwnerReferences[0].Name == sts.Name {
			stsPods = append(stsPods, &pod)
		}
	}

	if len(stsPods) > intendedReplicas {
		// We need to delete the pods..
		for i := len(stsPods) - 1; i > intendedReplicas; i-- {
			pod := stsPods[i]
			if err := r.Client.Delete(ctx, pod); err != nil {
				logger.Error(err, "Failed to delete extra pod from this StS")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	for i := 0; i < intendedReplicas; i++ {
		if i <= len(stsPods)-1 {
			continue
		}

		podKey := types.NamespacedName{
			Name:      fmt.Sprintf("%s-%d", sts.Name, i),
			Namespace: sts.Namespace,
		}

		pod := &corev1.Pod{
			// Technically this comes from a combination of Template.ObjectMeta, but we're just adding some fields
			ObjectMeta: metav1.ObjectMeta{
				Name:      podKey.Name,
				Namespace: podKey.Namespace,
				Labels:    sts.Spec.Template.Labels,
			},
			Spec: sts.Spec.Template.Spec,
		}

		// tbh, why do we need to add this here..?
		pod.Spec.Volumes = append(pod.Spec.Volumes,
			corev1.Volume{
				Name: "server-data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("server-data-%s", pod.Name),
					},
				},
			})

		if err := createPVC(ctx, r.Client, "server-data", pod); err != nil {
			logger.Error(err, "Failed to create PVC server-data", "Pod", podKey.Name)
			return ctrl.Result{}, err
		}

		// Create management API here and use its port..
		managementMockServer, err := FakeServer(r.Client, logger, podKey)
		if err != nil {
			logger.Error(err, "Failed to create fake mgmtApiServer")
			return ctrl.Result{}, err
		}
		ipPort := strings.Split(managementMockServer.Listener.Addr().String(), ":")
		port, err := strconv.Atoi(ipPort[1])
		if err != nil {
			logger.Error(err, "Failed to parse httptest server's port")
			return ctrl.Result{}, err
		}

		// TODO Change Liveness and Readiness ports also
		for c := 0; c < len(pod.Spec.Containers); c++ {
			if pod.Spec.Containers[c].Name == "cassandra" {
				for i := 0; i < len(pod.Spec.Containers[c].Ports); i++ {
					if pod.Spec.Containers[c].Ports[i].Name == "mgmt-api-http" {
						pod.Spec.Containers[c].Ports[i].ContainerPort = int32(port)
						break
					}
				}
				pod.Spec.Containers[c].LivenessProbe = probe(port, httphelper.LivenessEndpoint, 15, 15, 10)
				pod.Spec.Containers[c].ReadinessProbe = probe(port, httphelper.ReadinessEndpoint, 20, 10, 10)
				break
			}
		}

		if err := controllerutil.SetControllerReference(&sts, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Client.Create(context.TODO(), pod); err != nil {
			logger.Error(err, "Failed to create a Pod")
			return ctrl.Result{}, err
		}

		podIP := "127.0.0.1"
		patchPod := client.MergeFrom(pod.DeepCopy())
		pod.Status = corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "cassandra",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.NewTime(time.Now().Add(time.Second * -10)),
						},
					},
				},
			},
			Phase:  corev1.PodRunning,
			PodIP:  podIP,
			PodIPs: []corev1.PodIP{{IP: podIP}}}
		if err := r.Client.Status().Patch(ctx, pod, patchPod); err != nil {
			logger.Error(err, "Failed to patch the Pod")
			return ctrl.Result{}, err
		}
	}

	// Update StS status
	patchSts := client.MergeFrom(sts.DeepCopy())
	sts.Status.ReadyReplicas = int32(intendedReplicas)
	sts.Status.UpdatedReplicas = int32(intendedReplicas)
	sts.Status.AvailableReplicas = int32(intendedReplicas)
	sts.Status.CurrentReplicas = int32(intendedReplicas)
	sts.Status.Replicas = int32(intendedReplicas)
	sts.Status.ObservedGeneration = sts.Generation

	if err := r.Client.Status().Patch(ctx, &sts, patchSts); err != nil {
		logger.Error(err, "Failed to patch StatefulSet status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func probe(port int, path string, initDelay int, period int, timeout int) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(port),
				Path: path,
			},
		},
		InitialDelaySeconds: int32(initDelay),
		PeriodSeconds:       int32(period),
		TimeoutSeconds:      int32(timeout),
	}
}

// TODO These should be created to test certain decommission features also
func createPVC(ctx context.Context, cli client.Client, mountName string, pod *corev1.Pod) error {
	volumeMode := new(corev1.PersistentVolumeMode)
	*volumeMode = corev1.PersistentVolumeFilesystem
	storageClassName := "default"

	pvcName := types.NamespacedName{
		Name:      fmt.Sprintf("%s-%s", mountName, pod.Name),
		Namespace: pod.Namespace,
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName.Name,
			Namespace: pvcName.Namespace,
			Labels:    pod.Labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					// TODO Hardcoded not real value
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			StorageClassName: &storageClassName,
			VolumeMode:       volumeMode,
			VolumeName:       "pvc-" + pvcName.Name,
		},
	}

	return cli.Create(ctx, pvc)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
