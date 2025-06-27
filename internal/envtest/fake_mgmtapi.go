package envtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"

	"github.com/go-logr/logr"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	nodeCounter int = 0
	epMutex     sync.Mutex
	epData      httphelper.CassMetadataEndpoints
)

func FakeServer(cli client.Client, logger logr.Logger, podKey types.NamespacedName) (*httptest.Server, error) {
	// We're only interested in the PodName + PodNamespace..
	epMutex.Lock()
	myNodeName := strconv.Itoa(nodeCounter)
	if epData.Entity == nil {
		epData.Entity = make([]httphelper.EndpointState, 0, 1)
	}

	epData.Entity = append(epData.Entity, httphelper.EndpointState{
		HostID:                 myNodeName,
		EndpointIP:             "127.0.0.1",
		NativeAddressAndPort:   "127.0.0.1:9042",
		NativeTransportAddress: "127.0.0.1",
		ReleaseVersion:         "4.0.7",
		Status:                 string(httphelper.StatusNormal),
	})

	nodeCounter++

	epMutex.Unlock()

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}

		if r.Method == http.MethodPost {
			switch r.URL.Path {
			case "/api/v0/ops/seeds/reload":
				w.WriteHeader(http.StatusOK)
			case "/api/v0/lifecycle/start":
				logger.Info(fmt.Sprintf("Received call to start the pod.. %v", podKey))

				pod := &corev1.Pod{}
				if err := cli.Get(context.TODO(), podKey, pod); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				patchPod := client.MergeFrom(pod.DeepCopy())
				pod.Status.ContainerStatuses[0].Ready = true
				if err := cli.Status().Patch(context.TODO(), pod, patchPod); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusCreated)
			case "/api/v0/ops/auth/role":
				w.WriteHeader(http.StatusCreated)
			case "/api/v0/ops/node/decommission":
				epMutex.Lock()
				for i := 0; i < len(epData.Entity); i++ {
					if epData.Entity[i].HostID == myNodeName {
						epData.Entity[i].Status = string(httphelper.StatusLeft)
					}
				}
				epMutex.Unlock()
				w.WriteHeader(http.StatusOK)
			case "/api/v0/ops/keyspace/cleanup":
				w.WriteHeader(http.StatusOK)
			case "/api/v0/ops/node/encryption/internode/truststore/reload":
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		} else if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/api/v0/probes/cluster":
				// Health check call
				consistency := query.Get("consistency_level")
				rf := query.Get("rf_per_dc")
				if consistency != "LOCAL_QUORUM" || rf == "" {
					w.WriteHeader(http.StatusBadRequest)
					break
				}
				w.WriteHeader(http.StatusOK)
			case "/api/v0/metadata/endpoints":
				b, err := json.Marshal(&epData)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					break
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	managementMockServer := httptest.NewServer(handlerFunc)
	return managementMockServer, nil
}
