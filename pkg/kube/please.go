package kube

import "sigs.k8s.io/controller-runtime/pkg/client"

type Client interface {
	client.Client
}

type SubResourceClient interface {
	client.SubResourceClient
}
