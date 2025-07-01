// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

// This file defines constructors for k8s service-related objects
import (
	"fmt"
	"net"
	"strings"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Creates a headless service object for the Datacenter, for clients wanting to
// reach out to a ready Server node for either CQL or mgmt API
func newServiceForCassandraDatacenter(dc *api.CassandraDatacenter) *corev1.Service {
	svcName := dc.GetDatacenterServiceName()
	service := makeGenericHeadlessService(dc)
	service.Name = svcName

	nativePort := api.DefaultNativePort
	if dc.IsNodePortEnabled() {
		nativePort = dc.GetNodePortNativePort()
	}

	ports := []corev1.ServicePort{
		namedServicePort("native", nativePort, nativePort),
		namedServicePort("tls-native", 9142, 9142),
		namedServicePort("mgmt-api", 8080, 8080),
		namedServicePort("prometheus", 9103, 9103),
		namedServicePort("metrics", 9000, 9000),
	}

	if strings.HasPrefix(dc.Spec.ServerVersion, "3.") || dc.Spec.ServerType == "dse" {
		ports = append(ports,
			namedServicePort("thrift", 9160, 9160))
	}

	if dc.Spec.DseWorkloads != nil {
		if dc.Spec.DseWorkloads.AnalyticsEnabled {
			ports = append(
				ports,
				namedServicePort("dsefs-public", 5598, 5598),
				namedServicePort("spark-worker", 7081, 7081),
				namedServicePort("jobserver", 8090, 8090),
				namedServicePort("always-on-sql", 9077, 9077),
				namedServicePort("sql-thrift", 10000, 10000),
				namedServicePort("spark-history", 18080, 18080),
			)
		}

		if dc.Spec.DseWorkloads.GraphEnabled {
			ports = append(
				ports,
				namedServicePort("gremlin", 8182, 8182),
			)
		}

		if dc.Spec.DseWorkloads.SearchEnabled {
			ports = append(
				ports,
				namedServicePort("solr", 8983, 8983),
			)
		}
	}

	service.Spec.Ports = ports

	addAdditionalOptions(service, &dc.Spec.AdditionalServiceConfig.DatacenterService)

	utils.AddHashAnnotation(service)

	return service
}

func addAdditionalOptions(service *corev1.Service, serviceConfig *api.ServiceConfigAdditions) {
	if len(serviceConfig.Labels) > 0 {
		if service.Labels == nil {
			service.Labels = make(map[string]string, len(serviceConfig.Labels))
		}
		for k, v := range serviceConfig.Labels {
			service.Labels[k] = v
		}
	}

	if len(serviceConfig.Annotations) > 0 {
		if service.Annotations == nil {
			service.Annotations = make(map[string]string, len(serviceConfig.Annotations))
		}
		for k, v := range serviceConfig.Annotations {
			service.Annotations[k] = v
		}
	}
}

func namedServicePort(name string, port int, targetPort int) corev1.ServicePort {
	return corev1.ServicePort{Name: name, Port: int32(port), TargetPort: intstr.FromInt(targetPort)}
}

func buildLabelSelectorForSeedService(dc *api.CassandraDatacenter) map[string]string {
	labels := dc.GetClusterLabels()

	// narrow selection to just the seed nodes
	labels[api.SeedNodeLabel] = "true"

	return labels
}

// newSeedServiceForCassandraDatacenter creates a headless service owned by the CassandraDatacenter which will attach to all seed
// nodes in the cluster
func newSeedServiceForCassandraDatacenter(dc *api.CassandraDatacenter) *corev1.Service {
	service := makeGenericHeadlessService(dc)
	service.Name = dc.GetSeedServiceName()

	labels := dc.GetClusterLabels()
	oplabels.AddOperatorLabels(labels, dc)
	service.Labels = labels

	anns := dc.GetAnnotations()
	oplabels.AddOperatorAnnotations(anns, dc)
	service.Annotations = anns

	service.Spec.Selector = buildLabelSelectorForSeedService(dc)
	service.Spec.PublishNotReadyAddresses = true

	addAdditionalOptions(service, &dc.Spec.AdditionalServiceConfig.SeedService)

	utils.AddHashAnnotation(service)

	return service
}

// newAdditionalSeedServiceForCassandraDatacenter creates a headless service owned by the CassandraDatacenter,
// whether the additional seed pods are ready or not
func newAdditionalSeedServiceForCassandraDatacenter(dc *api.CassandraDatacenter) *corev1.Service {
	labels := dc.GetDatacenterLabels()
	oplabels.AddOperatorLabels(labels, dc)
	anns := make(map[string]string)
	oplabels.AddOperatorAnnotations(anns, dc)
	var service corev1.Service
	service.Name = dc.GetAdditionalSeedsServiceName()
	service.Namespace = dc.Namespace
	service.Labels = labels
	service.Annotations = anns
	// We omit the label selector because we will create the endpoints manually
	service.Spec.Type = "ClusterIP"
	service.Spec.ClusterIP = "None"
	service.Spec.PublishNotReadyAddresses = true

	addAdditionalOptions(&service, &dc.Spec.AdditionalServiceConfig.AdditionalSeedService)

	utils.AddHashAnnotation(&service)

	return &service
}

func newEndpointSlicesForAdditionalSeeds(dc *api.CassandraDatacenter) []*discoveryv1.EndpointSlice {
	ipv4Addresses := make([]string, 0)
	ipv6Addresses := make([]string, 0)
	fqdnAddresses := make([]string, 0)

	for _, additionalSeed := range dc.Spec.AdditionalSeeds {
		ip := net.ParseIP(additionalSeed)
		if ip != nil {
			if ip.To4() != nil {
				ipv4Addresses = append(ipv4Addresses, additionalSeed)
			} else {
				ipv6Addresses = append(ipv6Addresses, additionalSeed)
			}
		} else {
			fqdnAddresses = append(fqdnAddresses, additionalSeed)
		}
	}

	endpointSlices := make([]*discoveryv1.EndpointSlice, 0)

	ipv4Slice := createEndpointSlice(dc, discoveryv1.AddressTypeIPv4, ipv4Addresses)
	endpointSlices = append(endpointSlices, ipv4Slice)

	ipv6Slice := createEndpointSlice(dc, discoveryv1.AddressTypeIPv6, ipv6Addresses)
	endpointSlices = append(endpointSlices, ipv6Slice)

	fqdnSlice := createEndpointSlice(dc, discoveryv1.AddressTypeFQDN, fqdnAddresses)
	endpointSlices = append(endpointSlices, fqdnSlice)

	return endpointSlices
}

// Helper function to create an EndpointSlice of a specific address type
func createEndpointSlice(dc *api.CassandraDatacenter, addressType discoveryv1.AddressType, addresses []string) *discoveryv1.EndpointSlice {
	labels := dc.GetDatacenterLabels()
	oplabels.AddOperatorLabels(labels, dc)

	// Add service name as per Kubernetes EndpointSlice naming convention
	serviceName := dc.GetAdditionalSeedsServiceName()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[discoveryv1.LabelServiceName] = serviceName

	// Create a unique name based on service name and address type
	name := fmt.Sprintf("%s-%s", serviceName, strings.ToLower(string(addressType)))

	endpointSlice := discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: dc.Namespace,
			Labels:    labels,
		},
		AddressType: addressType,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: addresses,
			},
		},
	}

	anns := make(map[string]string)
	oplabels.AddOperatorAnnotations(anns, dc)
	endpointSlice.Annotations = anns

	utils.AddHashAnnotation(&endpointSlice)

	return &endpointSlice
}

// newNodePortServiceForCassandraDatacenter creates a headless service owned by the CassandraDatacenter,
// that preserves the client source IPs
func newNodePortServiceForCassandraDatacenter(dc *api.CassandraDatacenter) *corev1.Service {
	service := makeGenericHeadlessService(dc)
	service.Name = dc.GetNodePortServiceName()

	service.Spec.Type = "NodePort"
	// Note: ClusterIp = "None" is not valid for NodePort
	service.Spec.ClusterIP = ""
	service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal

	nativePort := dc.GetNodePortNativePort()
	internodePort := dc.GetNodePortInternodePort()

	service.Spec.Ports = []corev1.ServicePort{
		// Note: Port Names cannot be more than 15 characters
		{
			Name:       "internode",
			Port:       int32(internodePort),
			NodePort:   int32(internodePort),
			TargetPort: intstr.FromInt(internodePort),
		},
		{
			Name:       "native",
			Port:       int32(nativePort),
			NodePort:   int32(nativePort),
			TargetPort: intstr.FromInt(nativePort),
		},
	}

	addAdditionalOptions(service, &dc.Spec.AdditionalServiceConfig.NodePortService)
	return service
}

// newAllPodsServiceForCassandraDatacenter creates a headless service owned by the CassandraDatacenter,
// which covers all server pods in the datacenter, whether they are ready or not
func newAllPodsServiceForCassandraDatacenter(dc *api.CassandraDatacenter) *corev1.Service {
	service := makeGenericHeadlessService(dc)
	service.Name = dc.GetAllPodsServiceName()
	service.Labels[api.PromMetricsLabel] = "true"
	service.Spec.PublishNotReadyAddresses = true

	nativePort := api.DefaultNativePort
	if dc.IsNodePortEnabled() {
		nativePort = dc.GetNodePortNativePort()
	}

	service.Spec.Ports = []corev1.ServicePort{
		{
			Name: "native", Port: int32(nativePort), TargetPort: intstr.FromInt(nativePort),
		},
		{
			Name: "mgmt-api", Port: 8080, TargetPort: intstr.FromInt(8080),
		},
		{
			Name: "prometheus", Port: 9103, TargetPort: intstr.FromInt(9103),
		},
		{
			Name: "metrics", Port: 9000, TargetPort: intstr.FromInt(9000),
		},
	}

	addAdditionalOptions(service, &dc.Spec.AdditionalServiceConfig.AllPodsService)

	utils.AddHashAnnotation(service)

	return service
}

// makeGenericHeadlessService returns a fresh k8s headless (aka ClusterIP equals "None") Service
// struct that has the same namespace as the CassandraDatacenter argument, and proper labels for the DC.
// The caller needs to fill in the ObjectMeta.Name value, at a minimum, before it can be created
// inside the k8s cluster.
func makeGenericHeadlessService(dc *api.CassandraDatacenter) *corev1.Service {
	labels := dc.GetDatacenterLabels()
	oplabels.AddOperatorLabels(labels, dc)
	selector := dc.GetDatacenterLabels()
	var service corev1.Service
	service.Namespace = dc.Namespace
	service.Labels = labels
	service.Spec.Selector = selector
	service.Spec.Type = "ClusterIP"
	service.Spec.ClusterIP = "None"

	anns := make(map[string]string)
	oplabels.AddOperatorAnnotations(anns, dc)
	service.Annotations = anns

	return &service
}
