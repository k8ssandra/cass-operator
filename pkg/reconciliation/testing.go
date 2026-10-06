// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

//
// This file defines helpers for unit testing.
//

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	log2 "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	taskapi "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/images"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
	discoveryv1 "k8s.io/api/discovery/v1"
)

func newTestImageRegistry() images.ImageRegistry {
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing.yaml")
	registry, err := images.NewImageRegistry(imageConfigFile)
	if err != nil {
		panic(fmt.Sprintf("failed to create image registry: %v", err))
	}
	return registry
}

// MockSetControllerReference returns a method that will automatically reverse the mock
func MockSetControllerReference() func() {
	oldSetControllerReference := setControllerReference
	setControllerReference = func(
		owner,
		object metav1.Object,
		scheme *runtime.Scheme,
		opts ...controllerutil.OwnerReferenceOption,
	) error {
		return nil
	}

	return func() {
		setControllerReference = oldSetControllerReference
	}
}

// CreateMockReconciliationContext ...
func CreateMockReconciliationContext(
	reqLogger logr.Logger,
) *ReconciliationContext {
	// These defaults may need to be settable via arguments

	var (
		name              = "cassandradatacenter-example"
		clusterName       = "cassandradatacenter-example-cluster"
		namespace         = "default"
		size        int32 = 2
	)

	storageSize := resource.MustParse("1Gi")
	storageClassName := "standard"
	storageConfig := api.StorageConfig{
		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
			},
		},
	}

	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   storageClassName,
			Labels: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"},
		},
	}

	// Instance a cassandraDatacenter
	cassandraDatacenter := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: api.CassandraDatacenterSpec{
			Size:          size,
			ClusterName:   clusterName,
			ServerType:    "dse",
			ServerVersion: "6.8.4",
			StorageConfig: storageConfig,
		},
	}

	// Objects to keep track of

	trackObjects := []runtime.Object{
		cassandraDatacenter,
		storageClass,
	}

	s := setupScheme()
	// s.AddKnownTypes(api.GroupVersion, cassandraDatacenter)

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(cassandraDatacenter).WithRuntimeObjects(trackObjects...).Build()

	request := &reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	rc := &ReconciliationContext{}
	rc.Request = request
	rc.Client = fakeClient
	rc.Scheme = s
	rc.ReqLogger = reqLogger
	rc.Datacenter = cassandraDatacenter
	rc.Recorder = record.NewFakeRecorder(100)
	rc.Ctx = context.Background()
	rc.ImageRegistry = newTestImageRegistry()

	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("OK")),
	}

	mockHttpClient := mocks.NewHttpClient(&testing.T{})
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req != nil
			})).
		Return(res, nil)

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{Client: mockHttpClient, Log: reqLogger, Protocol: "http"}

	return rc
}

func setupTest() (*ReconciliationContext, *corev1.Service, func()) {
	// Set up verbose logging
	logger := zap.New()
	log2.SetLogger(logger)
	cleanupMockScr := MockSetControllerReference()

	rc := CreateMockReconciliationContext(logger)
	service := newServiceForCassandraDatacenter(rc.Datacenter)

	return rc, service, cleanupMockScr
}

func k8sMockClientGet(mockClient *mocks.Client, returnArg interface{}) *mock.Call {
	return mockClient.On("Get",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(key client.ObjectKey) bool {
				return key != client.ObjectKey{}
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			})).
		Return(returnArg).
		Once()
}

func k8sMockClientUpdate(mockClient *mocks.Client, returnArg interface{}) *mock.Call {
	return mockClient.On("Update",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			})).
		Return(returnArg).
		Once()
}

func k8sMockClientPatch(mockClient *mocks.Client, returnArg interface{}) *mock.Call {
	return mockClient.On("Patch",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			}),
		mock.MatchedBy(
			func(patch client.Patch) bool {
				return patch != nil
			})).
		Return(returnArg).
		Once()
}

func k8sMockClientStatusPatch(mockClient *mocks.SubResourceClient, returnArg interface{}) *mock.Call {
	return mockClient.On("Patch",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			}),
		mock.MatchedBy(
			func(patch client.Patch) bool {
				return patch != nil
			})).
		Return(returnArg).
		Once()
}

func k8sMockClientStatusUpdate(mockClient *mocks.SubResourceClient, returnArg interface{}) *mock.Call {
	return mockClient.On("Update",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			})).
		Return(returnArg).
		Once()
}

func k8sMockClientCreate(mockClient *mocks.Client, returnArg interface{}) *mock.Call {
	return mockClient.On("Create",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			})).
		Return(returnArg).
		Once()
}

func k8sMockClientDelete(mockClient *mocks.Client, returnArg interface{}) *mock.Call {
	return mockClient.On("Delete",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			})).
		Return(returnArg).
		Once()
}

func k8sMockClientList(mockClient *mocks.Client, returnArg interface{}) *mock.Call {
	return mockClient.On("List",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			}),
		mock.MatchedBy(matchListOptionsArg)).
		Return(returnArg).
		Once()
}

func matchListOptionsArg(arg interface{}) bool {
	return listOptionsFromArg(arg) != nil
}

func listOptionsFromArg(arg interface{}) *client.ListOptions {
	switch v := arg.(type) {
	case *client.ListOptions:
		return v
	case []client.ListOption:
		opts := &client.ListOptions{}
		for _, opt := range v {
			if opt != nil {
				opt.ApplyToList(opts)
			}
		}
		return opts
	default:
		return nil
	}
}

func init() {
	_ = clientgoscheme.AddToScheme(clientgoscheme.Scheme)
	_ = api.AddToScheme(clientgoscheme.Scheme)
	_ = taskapi.AddToScheme(clientgoscheme.Scheme)
	_ = corev1.AddToScheme(clientgoscheme.Scheme)
	_ = discoveryv1.AddToScheme(clientgoscheme.Scheme)
}

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = api.AddToScheme(scheme)
	_ = taskapi.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = discoveryv1.AddToScheme(scheme)
	return scheme
}

type callDetails struct {
	RequestPath string
	RequestURI  string
}

type fakeMgmtApiServer struct {
	server *httptest.Server

	mu    sync.Mutex
	calls []callDetails
}

func newFakeMgmtApiServer(t *testing.T, handler http.HandlerFunc) *fakeMgmtApiServer {
	server := startFakeMgmtApiTestServer(handler)
	t.Cleanup(server.Close)
	return server
}

func startFakeMgmtApiTestServer(handler http.HandlerFunc) *fakeMgmtApiServer {
	server := &fakeMgmtApiServer{}
	server.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.mu.Lock()
		server.calls = append(server.calls, callDetails{
			RequestPath: r.URL.Path,
			RequestURI:  r.URL.RequestURI(),
		})
		server.mu.Unlock()

		handler(w, r)
	}))
	return server
}

func (s *fakeMgmtApiServer) Close() {
	if s != nil && s.server != nil {
		s.server.Close()
	}
}

func (s *fakeMgmtApiServer) client(log logr.Logger) httphelper.NodeMgmtClient {
	protocol := "http"
	if strings.HasPrefix(s.server.URL, "https://") {
		protocol = "https"
	}

	return httphelper.NodeMgmtClient{
		Client:   s.server.Client(),
		Log:      log,
		Protocol: protocol,
	}
}

func (s *fakeMgmtApiServer) attachToPod(t *testing.T, pod *corev1.Pod) {
	host, port := s.hostPort(t)
	pod.Status.PodIP = host

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Name != "cassandra" {
			continue
		}
		for j := range container.Ports {
			if container.Ports[j].Name == "mgmt-api-http" {
				container.Ports[j].ContainerPort = int32(port)
				return
			}
		}
		container.Ports = append(container.Ports, corev1.ContainerPort{Name: "mgmt-api-http", ContainerPort: int32(port)})
		return
	}
}

func (s *fakeMgmtApiServer) hostPort(t *testing.T) (string, int) {
	t.Helper()

	addr := strings.TrimPrefix(strings.TrimPrefix(s.server.URL, "http://"), "https://")
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	return host, port
}

func (s *fakeMgmtApiServer) callCount(requestURI string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, call := range s.calls {
		if strings.Contains(requestURI, "?") {
			if call.RequestURI == requestURI {
				count++
			}
			continue
		}
		if call.RequestPath == requestURI {
			count++
		}
	}

	return count
}

func (s *fakeMgmtApiServer) assertCallCount(t *testing.T, requestURI string, expected int) {
	t.Helper() // To ensure the line number reported on failure is the caller of this method
	require.Equal(t, expected, s.callCount(requestURI), "expected %d calls to %s", expected, requestURI)
}
