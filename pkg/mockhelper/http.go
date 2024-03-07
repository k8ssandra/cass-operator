package mockhelper

import (
	"net/http"

	client "sigs.k8s.io/controller-runtime/pkg/client"
)

type Client interface {
	client.Client
}

type HttpClient interface {
	http.Client
}
