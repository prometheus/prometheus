// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "kubernetes_"

	// roleLabel is the name for the label containing a target's role.
	roleLabel = metaLabelPrefix + "role"

	sourcePodPrefix = "pods"
	// podsTargetGroupNAme is the name given to the target group for pods
	podsTargetGroupName = "pods"
	// podNamespaceLabel is the name for the label containing a target pod's namespace
	podNamespaceLabel = metaLabelPrefix + "pod_namespace"
	// podNameLabel is the name for the label containing a target pod's name
	podNameLabel = metaLabelPrefix + "pod_name"
	// podAddressLabel is the name for the label containing a target pod's IP address (the PodIP)
	podAddressLabel = metaLabelPrefix + "pod_address"
	// podContainerNameLabel is the name for the label containing a target's container name
	podContainerNameLabel = metaLabelPrefix + "pod_container_name"
	// podContainerPortNameLabel is the name for the label containing the name of the port selected for a target
	podContainerPortNameLabel = metaLabelPrefix + "pod_container_port_name"
	// PodContainerPortListLabel is the name for the label containing a list of all TCP ports on the target container
	podContainerPortListLabel = metaLabelPrefix + "pod_container_port_list"
	// PodContainerPortMapPrefix is the prefix used to create the names of labels that associate container port names to port values
	// Such labels will be named (podContainerPortMapPrefix)_(PortName) = (ContainerPort)
	podContainerPortMapPrefix = metaLabelPrefix + "pod_container_port_map_"
	// podReadyLabel is the name for the label containing the 'Ready' status (true/false/unknown) for a target
	podReadyLabel = metaLabelPrefix + "pod_ready"
	// podLabelPrefix is the prefix for prom label names corresponding to k8s labels for a target pod
	podLabelPrefix = metaLabelPrefix + "pod_label_"
	// podAnnotationPrefix is the prefix for prom label names corresponding to k8s annotations for a target pod
	podAnnotationPrefix = metaLabelPrefix + "pod_annotation_"

	sourceServicePrefix = "services"
	// serviceNamespaceLabel is the name for the label containing a target's service namespace.
	serviceNamespaceLabel = metaLabelPrefix + "service_namespace"
	// serviceNameLabel is the name for the label containing a target's service name.
	serviceNameLabel = metaLabelPrefix + "service_name"
	// serviceLabelPrefix is the prefix for the service labels.
	serviceLabelPrefix = metaLabelPrefix + "service_label_"
	// serviceAnnotationPrefix is the prefix for the service annotations.
	serviceAnnotationPrefix = metaLabelPrefix + "service_annotation_"

	// nodesTargetGroupName is the name given to the target group for nodes.
	nodesTargetGroupName = "nodes"
	// nodeLabelPrefix is the prefix for the node labels.
	nodeLabelPrefix = metaLabelPrefix + "node_label_"

	// apiServersTargetGroupName is the name given to the target group for API servers.
	apiServersTargetGroupName = "apiServers"

	serviceAccountToken  = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	serviceAccountCACert = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

	apiVersion          = "v1"
	apiPrefix           = "/api/" + apiVersion
	nodesURL            = apiPrefix + "/nodes"
	podsURL             = apiPrefix + "/pods"
	servicesURL         = apiPrefix + "/services"
	endpointsURL        = apiPrefix + "/endpoints"
	serviceEndpointsURL = apiPrefix + "/namespaces/%s/endpoints/%s"
)

// Discovery implements a TargetProvider for Kubernetes services.
type Discovery struct {
	client *http.Client
	Conf   *config.KubernetesSDConfig

	apiServers   []config.URL
	apiServersMu sync.RWMutex
	nodes        map[string]*Node
	services     map[string]map[string]*Service
	// map of namespace to (map of pod name to pod)
	pods       map[string]map[string]*Pod
	nodesMu    sync.RWMutex
	servicesMu sync.RWMutex
	podsMu     sync.RWMutex
	runDone    chan struct{}
}

// Initialize sets up the discovery for usage.
func (kd *Discovery) Initialize() error {
	client, err := newKubernetesHTTPClient(kd.Conf)

	if err != nil {
		return err
	}

	kd.apiServers = kd.Conf.APIServers
	kd.client = client
	kd.runDone = make(chan struct{})

	return nil
}

// Run implements the TargetProvider interface.
func (kd *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	log.Debugf("Kubernetes Discovery.Run beginning")
	defer close(ch)

	// Send an initial full view.
	// TODO(fabxc): this does not include all available services and service
	// endpoints yet. Service endpoints were also missing in the previous Sources() method.
	var all []*config.TargetGroup

	all = append(all, kd.updateAPIServersTargetGroup())
	all = append(all, kd.updateNodesTargetGroup())
	all = append(all, kd.updatePodsTargetGroup())
	for _, ns := range kd.pods {
		for _, pod := range ns {
			all = append(all, kd.updatePodTargetGroup(pod))
		}
	}

	select {
	case ch <- all:
	case <-ctx.Done():
		return
	}

	retryInterval := time.Duration(kd.Conf.RetryInterval)

	update := make(chan interface{}, 10)

	go kd.watchNodes(update, ctx.Done(), retryInterval)
	go kd.startServiceWatch(update, ctx.Done(), retryInterval)
	go kd.watchPods(update, ctx.Done(), retryInterval)

	for {
		tg := []*config.TargetGroup{}
		select {
		case <-ctx.Done():
			return
		case event := <-update:
			switch obj := event.(type) {
			case *nodeEvent:
				log.Debugf("k8s discovery received node event (EventType=%s, Node Name=%s)", obj.EventType, obj.Node.ObjectMeta.Name)
				kd.updateNode(obj.Node, obj.EventType)
				tg = append(tg, kd.updateNodesTargetGroup())
			case *serviceEvent:
				log.Debugf("k8s discovery received service event (EventType=%s, Service Name=%s)", obj.EventType, obj.Service.ObjectMeta.Name)
				tg = append(tg, kd.updateService(obj.Service, obj.EventType))
			case *endpointsEvent:
				log.Debugf("k8s discovery received endpoint event (EventType=%s, Endpoint Name=%s)", obj.EventType, obj.Endpoints.ObjectMeta.Name)
				tg = append(tg, kd.updateServiceEndpoints(obj.Endpoints, obj.EventType))
			case *podEvent:
				log.Debugf("k8s discovery received pod event (EventType=%s, Pod Name=%s)", obj.EventType, obj.Pod.ObjectMeta.Name)
				// Update the per-pod target group
				kd.updatePod(obj.Pod, obj.EventType)
				tg = append(tg, kd.updatePodTargetGroup(obj.Pod))
				// ...and update the all pods target group
				tg = append(tg, kd.updatePodsTargetGroup())
			}
		}

		if tg == nil {
			continue
		}

		for _, t := range tg {
			select {
			case ch <- []*config.TargetGroup{t}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (kd *Discovery) queryAPIServerPath(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	return kd.queryAPIServerReq(req)
}

func (kd *Discovery) queryAPIServerReq(req *http.Request) (*http.Response, error) {
	// Lock in case we need to rotate API servers to request.
	kd.apiServersMu.Lock()
	defer kd.apiServersMu.Unlock()
	var lastErr error
	for i := 0; i < len(kd.apiServers); i++ {
		cloneReq := *req
		cloneReq.URL.Host = kd.apiServers[0].Host
		cloneReq.URL.Scheme = kd.apiServers[0].Scheme
		res, err := kd.client.Do(&cloneReq)
		if err == nil {
			return res, nil
		}
		lastErr = err
		kd.rotateAPIServers()
	}
	return nil, fmt.Errorf("unable to query any API servers: %v", lastErr)
}

func (kd *Discovery) rotateAPIServers() {
	if len(kd.apiServers) > 1 {
		kd.apiServers = append(kd.apiServers[1:], kd.apiServers[0])
	}
}

func (kd *Discovery) updateAPIServersTargetGroup() *config.TargetGroup {
	tg := &config.TargetGroup{
		Source: apiServersTargetGroupName,
		Labels: model.LabelSet{
			roleLabel: model.LabelValue("apiserver"),
		},
	}

	for _, apiServer := range kd.apiServers {
		apiServerAddress := apiServer.Host
		_, _, err := net.SplitHostPort(apiServerAddress)
		// If error then no port is specified - use default for scheme.
		if err != nil {
			switch apiServer.Scheme {
			case "http":
				apiServerAddress = net.JoinHostPort(apiServerAddress, "80")
			case "https":
				apiServerAddress = net.JoinHostPort(apiServerAddress, "443")
			}
		}

		t := model.LabelSet{
			model.AddressLabel: model.LabelValue(apiServerAddress),
			model.SchemeLabel:  model.LabelValue(apiServer.Scheme),
		}
		tg.Targets = append(tg.Targets, t)
	}

	return tg
}

func (kd *Discovery) updateNodesTargetGroup() *config.TargetGroup {
	kd.nodesMu.RLock()
	defer kd.nodesMu.RUnlock()

	tg := &config.TargetGroup{
		Source: nodesTargetGroupName,
		Labels: model.LabelSet{
			roleLabel: model.LabelValue("node"),
		},
	}

	// Now let's loop through the nodes & add them to the target group with appropriate labels.
	for nodeName, node := range kd.nodes {
		nodeAddress, err := nodeHostIP(node)
		if err != nil {
			log.Debugf("Skipping node %s: %s", node.Name, err)
			continue
		}

		address := fmt.Sprintf("%s:%d", nodeAddress.String(), kd.Conf.KubeletPort)

		t := model.LabelSet{
			model.AddressLabel:  model.LabelValue(address),
			model.InstanceLabel: model.LabelValue(nodeName),
		}
		for k, v := range node.ObjectMeta.Labels {
			labelName := strutil.SanitizeLabelName(nodeLabelPrefix + k)
			t[model.LabelName(labelName)] = model.LabelValue(v)
		}
		tg.Targets = append(tg.Targets, t)
	}

	return tg
}

func (kd *Discovery) updateNode(node *Node, eventType EventType) {
	kd.nodesMu.Lock()
	defer kd.nodesMu.Unlock()
	updatedNodeName := node.ObjectMeta.Name
	switch eventType {
	case Deleted:
		// Deleted - remove from nodes map.
		delete(kd.nodes, updatedNodeName)
	case Added, Modified:
		// Added/Modified - update the node in the nodes map.
		kd.nodes[updatedNodeName] = node
	}
}

func (kd *Discovery) getNodes() (map[string]*Node, string, error) {
	res, err := kd.queryAPIServerPath(nodesURL)
	if err != nil {
		// If we can't list nodes then we can't watch them. Assume this is a misconfiguration
		// & return error.
		return nil, "", fmt.Errorf("unable to list Kubernetes nodes: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unable to list Kubernetes nodes; unexpected response: %d %s", res.StatusCode, res.Status)
	}

	var nodes NodeList
	if err := json.NewDecoder(res.Body).Decode(&nodes); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, "", fmt.Errorf("unable to list Kubernetes nodes; unexpected response body: %s", string(body))
	}

	nodeMap := map[string]*Node{}
	for idx, node := range nodes.Items {
		nodeMap[node.ObjectMeta.Name] = &nodes.Items[idx]
	}

	return nodeMap, nodes.ResourceVersion, nil
}

func (kd *Discovery) getServices() (map[string]map[string]*Service, string, error) {
	res, err := kd.queryAPIServerPath(servicesURL)
	if err != nil {
		// If we can't list services then we can't watch them. Assume this is a misconfiguration
		// & return error.
		return nil, "", fmt.Errorf("unable to list Kubernetes services: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unable to list Kubernetes services; unexpected response: %d %s", res.StatusCode, res.Status)
	}
	var services ServiceList
	if err := json.NewDecoder(res.Body).Decode(&services); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, "", fmt.Errorf("unable to list Kubernetes services; unexpected response body: %s", string(body))
	}

	serviceMap := map[string]map[string]*Service{}
	for idx, service := range services.Items {
		namespace, ok := serviceMap[service.ObjectMeta.Namespace]
		if !ok {
			namespace = map[string]*Service{}
			serviceMap[service.ObjectMeta.Namespace] = namespace
		}
		namespace[service.ObjectMeta.Name] = &services.Items[idx]
	}

	return serviceMap, services.ResourceVersion, nil
}

// watchNodes watches nodes as they come & go.
func (kd *Discovery) watchNodes(events chan interface{}, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		nodes, resourceVersion, err := kd.getNodes()
		if err != nil {
			log.Errorf("Cannot initialize nodes collection: %s", err)
			return
		}

		// Reset the known nodes.
		kd.nodes = map[string]*Node{}

		for _, node := range nodes {
			events <- &nodeEvent{Added, node}
		}

		req, err := http.NewRequest("GET", nodesURL, nil)
		if err != nil {
			log.Errorf("Cannot create nodes request: %s", err)
			return
		}
		values := req.URL.Query()
		values.Add("watch", "true")
		values.Add("resourceVersion", resourceVersion)
		req.URL.RawQuery = values.Encode()
		res, err := kd.queryAPIServerReq(req)
		if err != nil {
			log.Errorf("Failed to watch nodes: %s", err)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			log.Errorf("Failed to watch nodes: %d", res.StatusCode)
			return
		}

		d := json.NewDecoder(res.Body)

		for {
			var event nodeEvent
			if err := d.Decode(&event); err != nil {
				log.Errorf("Watch nodes unexpectedly closed: %s", err)
				return
			}

			select {
			case events <- &event:
			case <-done:
			}
		}
	}, retryInterval, done)
}

// watchServices watches services as they come & go.
func (kd *Discovery) startServiceWatch(events chan<- interface{}, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		// We use separate target groups for each discovered service so we'll need to clean up any if they've been deleted
		// in Kubernetes while we couldn't connect - small chance of this, but worth dealing with.
		existingServices := kd.services

		// Reset the known services.
		kd.services = map[string]map[string]*Service{}

		services, resourceVersion, err := kd.getServices()
		if err != nil {
			log.Errorf("Cannot initialize services collection: %s", err)
			return
		}

		// Now let's loop through the old services & see if they still exist in here
		for oldNSName, oldNS := range existingServices {
			if ns, ok := services[oldNSName]; !ok {
				for _, service := range existingServices[oldNSName] {
					events <- &serviceEvent{Deleted, service}
				}
			} else {
				for oldServiceName, oldService := range oldNS {
					if _, ok := ns[oldServiceName]; !ok {
						events <- &serviceEvent{Deleted, oldService}
					}
				}
			}
		}

		// Discard the existing services map for GC.
		existingServices = nil

		for _, ns := range services {
			for _, service := range ns {
				events <- &serviceEvent{Added, service}
			}
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			kd.watchServices(resourceVersion, events, done)
			wg.Done()
		}()
		go func() {
			kd.watchServiceEndpoints(resourceVersion, events, done)
			wg.Done()
		}()

		wg.Wait()
	}, retryInterval, done)
}

func (kd *Discovery) watchServices(resourceVersion string, events chan<- interface{}, done <-chan struct{}) {
	req, err := http.NewRequest("GET", servicesURL, nil)
	if err != nil {
		log.Errorf("Failed to create services request: %s", err)
		return
	}
	values := req.URL.Query()
	values.Add("watch", "true")
	values.Add("resourceVersion", resourceVersion)
	req.URL.RawQuery = values.Encode()

	res, err := kd.queryAPIServerReq(req)
	if err != nil {
		log.Errorf("Failed to watch services: %s", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Failed to watch services: %d", res.StatusCode)
		return
	}

	d := json.NewDecoder(res.Body)

	for {
		var event serviceEvent
		if err := d.Decode(&event); err != nil {
			log.Errorf("Watch services unexpectedly closed: %s", err)
			return
		}

		select {
		case events <- &event:
		case <-done:
			return
		}
	}
}

// watchServiceEndpoints watches service endpoints as they come & go.
func (kd *Discovery) watchServiceEndpoints(resourceVersion string, events chan<- interface{}, done <-chan struct{}) {
	req, err := http.NewRequest("GET", endpointsURL, nil)
	if err != nil {
		log.Errorf("Failed to create service endpoints request: %s", err)
		return
	}
	values := req.URL.Query()
	values.Add("watch", "true")
	values.Add("resourceVersion", resourceVersion)
	req.URL.RawQuery = values.Encode()

	res, err := kd.queryAPIServerReq(req)
	if err != nil {
		log.Errorf("Failed to watch service endpoints: %s", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Failed to watch service endpoints: %d", res.StatusCode)
		return
	}

	d := json.NewDecoder(res.Body)

	for {
		var event endpointsEvent
		if err := d.Decode(&event); err != nil {
			log.Errorf("Watch service endpoints unexpectedly closed: %s", err)
			return
		}

		select {
		case events <- &event:
		case <-done:
		}
	}
}

func (kd *Discovery) updateService(service *Service, eventType EventType) *config.TargetGroup {
	kd.servicesMu.Lock()
	defer kd.servicesMu.Unlock()

	switch eventType {
	case Deleted:
		return kd.deleteService(service)
	case Added, Modified:
		return kd.addService(service)
	}
	return nil
}

func (kd *Discovery) deleteService(service *Service) *config.TargetGroup {
	tg := &config.TargetGroup{Source: serviceSource(service)}

	delete(kd.services[service.ObjectMeta.Namespace], service.ObjectMeta.Name)
	if len(kd.services[service.ObjectMeta.Namespace]) == 0 {
		delete(kd.services, service.ObjectMeta.Namespace)
	}

	return tg
}

func (kd *Discovery) addService(service *Service) *config.TargetGroup {
	namespace, ok := kd.services[service.ObjectMeta.Namespace]
	if !ok {
		namespace = map[string]*Service{}
		kd.services[service.ObjectMeta.Namespace] = namespace
	}

	namespace[service.ObjectMeta.Name] = service
	endpointURL := fmt.Sprintf(serviceEndpointsURL, service.ObjectMeta.Namespace, service.ObjectMeta.Name)

	res, err := kd.queryAPIServerPath(endpointURL)
	if err != nil {
		log.Errorf("Error getting service endpoints: %s", err)
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Failed to get service endpoints: %d", res.StatusCode)
		return nil
	}

	var eps Endpoints
	if err := json.NewDecoder(res.Body).Decode(&eps); err != nil {
		log.Errorf("Error getting service endpoints: %s", err)
		return nil
	}

	return kd.updateServiceTargetGroup(service, &eps)
}

func (kd *Discovery) updateServiceTargetGroup(service *Service, eps *Endpoints) *config.TargetGroup {
	tg := &config.TargetGroup{
		Source: serviceSource(service),
		Labels: model.LabelSet{
			serviceNamespaceLabel: model.LabelValue(service.ObjectMeta.Namespace),
			serviceNameLabel:      model.LabelValue(service.ObjectMeta.Name),
		},
	}

	for k, v := range service.ObjectMeta.Labels {
		labelName := strutil.SanitizeLabelName(serviceLabelPrefix + k)
		tg.Labels[model.LabelName(labelName)] = model.LabelValue(v)
	}

	for k, v := range service.ObjectMeta.Annotations {
		labelName := strutil.SanitizeLabelName(serviceAnnotationPrefix + k)
		tg.Labels[model.LabelName(labelName)] = model.LabelValue(v)
	}

	serviceAddress := service.ObjectMeta.Name + "." + service.ObjectMeta.Namespace + ".svc"

	// Append the first TCP service port if one exists.
	for _, port := range service.Spec.Ports {
		if port.Protocol == ProtocolTCP {
			serviceAddress += fmt.Sprintf(":%d", port.Port)
			break
		}
	}

	t := model.LabelSet{
		model.AddressLabel: model.LabelValue(serviceAddress),
		roleLabel:          model.LabelValue("service"),
	}
	tg.Targets = append(tg.Targets, t)

	// Now let's loop through the endpoints & add them to the target group with appropriate labels.
	for _, ss := range eps.Subsets {
		epPort := ss.Ports[0].Port

		for _, addr := range ss.Addresses {
			ipAddr := addr.IP
			if len(ipAddr) == net.IPv6len {
				ipAddr = "[" + ipAddr + "]"
			}
			address := fmt.Sprintf("%s:%d", ipAddr, epPort)

			t := model.LabelSet{
				model.AddressLabel: model.LabelValue(address),
				roleLabel:          model.LabelValue("endpoint"),
			}

			tg.Targets = append(tg.Targets, t)
		}
	}

	return tg
}

func (kd *Discovery) updateServiceEndpoints(endpoints *Endpoints, eventType EventType) *config.TargetGroup {
	kd.servicesMu.Lock()
	defer kd.servicesMu.Unlock()

	serviceNamespace := endpoints.ObjectMeta.Namespace
	serviceName := endpoints.ObjectMeta.Name

	if service, ok := kd.services[serviceNamespace][serviceName]; ok {
		return kd.updateServiceTargetGroup(service, endpoints)
	}
	return nil
}

func newKubernetesHTTPClient(conf *config.KubernetesSDConfig) (*http.Client, error) {
	bearerTokenFile := conf.BearerTokenFile
	caFile := conf.TLSConfig.CAFile
	if conf.InCluster {
		if len(bearerTokenFile) == 0 {
			bearerTokenFile = serviceAccountToken
		}
		if len(caFile) == 0 {
			// With recent versions, the CA certificate is mounted as a secret
			// but we need to handle older versions too. In this case, don't
			// set the CAFile & the configuration will have to use InsecureSkipVerify.
			if _, err := os.Stat(serviceAccountCACert); err == nil {
				caFile = serviceAccountCACert
			}
		}
	}

	tlsOpts := httputil.TLSOptions{
		InsecureSkipVerify: conf.TLSConfig.InsecureSkipVerify,
		CAFile:             caFile,
		CertFile:           conf.TLSConfig.CertFile,
		KeyFile:            conf.TLSConfig.KeyFile,
	}
	tlsConfig, err := httputil.NewTLSConfig(tlsOpts)
	if err != nil {
		return nil, err
	}

	var rt http.RoundTripper = &http.Transport{
		Dial: func(netw, addr string) (c net.Conn, err error) {
			c, err = net.DialTimeout(netw, addr, time.Duration(conf.RequestTimeout))
			return
		},
		TLSClientConfig: tlsConfig,
	}

	// If a bearer token is provided, create a round tripper that will set the
	// Authorization header correctly on each request.
	bearerToken := conf.BearerToken
	if len(bearerToken) == 0 && len(bearerTokenFile) > 0 {
		b, err := ioutil.ReadFile(bearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read bearer token file %s: %s", bearerTokenFile, err)
		}
		bearerToken = string(b)
	}
	if len(bearerToken) > 0 {
		rt = httputil.NewBearerAuthRoundTripper(bearerToken, rt)
	}

	if conf.BasicAuth != nil {
		rt = httputil.NewBasicAuthRoundTripper(conf.BasicAuth.Username, conf.BasicAuth.Password, rt)
	}

	return &http.Client{
		Transport: rt,
	}, nil
}

func serviceSource(service *Service) string {
	return sourceServicePrefix + ":" + service.ObjectMeta.Namespace + "/" + service.ObjectMeta.Name
}

// Until loops until stop channel is closed, running f every period.
// f may not be invoked if stop channel is already closed.
func until(f func(), period time.Duration, stopCh <-chan struct{}) {
	select {
	case <-stopCh:
		return
	default:
		f()
	}
	for {
		select {
		case <-stopCh:
			return
		case <-time.After(period):
			f()
		}
	}
}

// nodeHostIP returns the provided node's address, based on the priority:
// 1. NodeInternalIP
// 2. NodeExternalIP
// 3. NodeLegacyHostIP
//
// Copied from k8s.io/kubernetes/pkg/util/node/node.go
func nodeHostIP(node *Node) (net.IP, error) {
	addresses := node.Status.Addresses
	addressMap := make(map[NodeAddressType][]NodeAddress)
	for i := range addresses {
		addressMap[addresses[i].Type] = append(addressMap[addresses[i].Type], addresses[i])
	}
	if addresses, ok := addressMap[NodeInternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	if addresses, ok := addressMap[NodeExternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	if addresses, ok := addressMap[NodeLegacyHostIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	return nil, fmt.Errorf("host IP unknown; known addresses: %v", addresses)
}

////////////////////////////////////
// Here there be dragons.         //
// Pod discovery code lies below. //
////////////////////////////////////

func (kd *Discovery) updatePod(pod *Pod, eventType EventType) {
	kd.podsMu.Lock()
	defer kd.podsMu.Unlock()

	switch eventType {
	case Deleted:
		if _, ok := kd.pods[pod.ObjectMeta.Namespace]; ok {
			delete(kd.pods[pod.ObjectMeta.Namespace], pod.ObjectMeta.Name)
			if len(kd.pods[pod.ObjectMeta.Namespace]) == 0 {
				delete(kd.pods, pod.ObjectMeta.Namespace)
			}
		}
	case Added, Modified:
		if _, ok := kd.pods[pod.ObjectMeta.Namespace]; !ok {
			kd.pods[pod.ObjectMeta.Namespace] = map[string]*Pod{}
		}
		kd.pods[pod.ObjectMeta.Namespace][pod.ObjectMeta.Name] = pod
	}
}

func (kd *Discovery) getPods() (map[string]map[string]*Pod, string, error) {
	res, err := kd.queryAPIServerPath(podsURL)
	if err != nil {
		return nil, "", fmt.Errorf("unable to list Kubernetes pods: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unable to list Kubernetes pods; unexpected response: %d %s", res.StatusCode, res.Status)
	}

	var pods PodList
	if err := json.NewDecoder(res.Body).Decode(&pods); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, "", fmt.Errorf("unable to list Kubernetes pods; unexpected response body: %s", string(body))
	}

	podMap := map[string]map[string]*Pod{}
	for idx, pod := range pods.Items {
		if _, ok := podMap[pod.ObjectMeta.Namespace]; !ok {
			podMap[pod.ObjectMeta.Namespace] = map[string]*Pod{}
		}
		log.Debugf("Got pod %s in namespace %s", pod.ObjectMeta.Name, pod.ObjectMeta.Namespace)
		podMap[pod.ObjectMeta.Namespace][pod.ObjectMeta.Name] = &pods.Items[idx]
	}

	return podMap, pods.ResourceVersion, nil
}

func (kd *Discovery) watchPods(events chan interface{}, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		pods, resourceVersion, err := kd.getPods()
		if err != nil {
			log.Errorf("Cannot initialize pods collection: %s", err)
			return
		}
		kd.podsMu.Lock()
		kd.pods = pods
		kd.podsMu.Unlock()

		req, err := http.NewRequest("GET", podsURL, nil)
		if err != nil {
			log.Errorf("Cannot create pods request: %s", err)
			return
		}

		values := req.URL.Query()
		values.Add("watch", "true")
		values.Add("resourceVersion", resourceVersion)
		req.URL.RawQuery = values.Encode()
		res, err := kd.queryAPIServerReq(req)
		if err != nil {
			log.Errorf("Failed to watch pods: %s", err)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			log.Errorf("Failed to watch pods: %d", res.StatusCode)
			return
		}

		d := json.NewDecoder(res.Body)

		for {
			var event podEvent
			if err := d.Decode(&event); err != nil {
				log.Errorf("Watch pods unexpectedly closed: %s", err)
				return
			}

			select {
			case events <- &event:
			case <-done:
			}
		}
	}, retryInterval, done)
}

func podSource(pod *Pod) string {
	return sourcePodPrefix + ":" + pod.ObjectMeta.Namespace + ":" + pod.ObjectMeta.Name
}

type ByContainerPort []ContainerPort

func (a ByContainerPort) Len() int           { return len(a) }
func (a ByContainerPort) Less(i, j int) bool { return a[i].ContainerPort < a[j].ContainerPort }
func (a ByContainerPort) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

type ByContainerName []Container

func (a ByContainerName) Len() int           { return len(a) }
func (a ByContainerName) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a ByContainerName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func updatePodTargets(pod *Pod, allContainers bool) []model.LabelSet {
	var targets []model.LabelSet = make([]model.LabelSet, 0, len(pod.PodSpec.Containers))
	if pod.PodStatus.PodIP == "" {
		log.Debugf("skipping pod %s -- PodStatus.PodIP is empty", pod.ObjectMeta.Name)
		return targets
	}

	if pod.PodStatus.Phase != "Running" {
		log.Debugf("skipping pod %s -- status is not `Running`", pod.ObjectMeta.Name)
		return targets
	}

	ready := "unknown"
	for _, cond := range pod.PodStatus.Conditions {
		if strings.ToLower(cond.Type) == "ready" {
			ready = strings.ToLower(cond.Status)
		}
	}

	sort.Sort(ByContainerName(pod.PodSpec.Containers))

	for _, container := range pod.PodSpec.Containers {
		// Collect a list of TCP ports
		// Sort by port number, ascending
		// Product a target pointed at the first port
		// Include a label containing all ports (portName=port,PortName=port,...,)
		var tcpPorts []ContainerPort
		var portLabel *bytes.Buffer = bytes.NewBufferString(",")

		for _, port := range container.Ports {
			if port.Protocol == "TCP" {
				tcpPorts = append(tcpPorts, port)
			}
		}

		if len(tcpPorts) == 0 {
			log.Debugf("skipping container %s with no TCP ports", container.Name)
			continue
		}

		sort.Sort(ByContainerPort(tcpPorts))

		t := model.LabelSet{
			model.AddressLabel:        model.LabelValue(net.JoinHostPort(pod.PodIP, strconv.FormatInt(int64(tcpPorts[0].ContainerPort), 10))),
			podNameLabel:              model.LabelValue(pod.ObjectMeta.Name),
			podAddressLabel:           model.LabelValue(pod.PodStatus.PodIP),
			podNamespaceLabel:         model.LabelValue(pod.ObjectMeta.Namespace),
			podContainerNameLabel:     model.LabelValue(container.Name),
			podContainerPortNameLabel: model.LabelValue(tcpPorts[0].Name),
			podReadyLabel:             model.LabelValue(ready),
		}

		for _, port := range tcpPorts {
			portLabel.WriteString(port.Name)
			portLabel.WriteString("=")
			portLabel.WriteString(strconv.FormatInt(int64(port.ContainerPort), 10))
			portLabel.WriteString(",")
			t[model.LabelName(podContainerPortMapPrefix+port.Name)] = model.LabelValue(strconv.FormatInt(int64(port.ContainerPort), 10))
		}

		t[model.LabelName(podContainerPortListLabel)] = model.LabelValue(portLabel.String())

		for k, v := range pod.ObjectMeta.Labels {
			labelName := strutil.SanitizeLabelName(podLabelPrefix + k)
			t[model.LabelName(labelName)] = model.LabelValue(v)
		}

		for k, v := range pod.ObjectMeta.Annotations {
			labelName := strutil.SanitizeLabelName(podAnnotationPrefix + k)
			t[model.LabelName(labelName)] = model.LabelValue(v)
		}

		targets = append(targets, t)

		if !allContainers {
			break
		}
	}
	return targets
}

func (kd *Discovery) updatePodTargetGroup(pod *Pod) *config.TargetGroup {
	kd.podsMu.RLock()
	defer kd.podsMu.RUnlock()

	tg := &config.TargetGroup{
		Source: podSource(pod),
	}

	// If this pod doesn't exist, return an empty target group
	if _, ok := kd.pods[pod.ObjectMeta.Namespace]; !ok {
		return tg
	}
	if _, ok := kd.pods[pod.ObjectMeta.Namespace][pod.ObjectMeta.Name]; !ok {
		return tg
	}

	tg.Labels = model.LabelSet{
		roleLabel: model.LabelValue("container"),
	}
	tg.Targets = updatePodTargets(pod, true)

	return tg
}

func (kd *Discovery) updatePodsTargetGroup() *config.TargetGroup {
	tg := &config.TargetGroup{
		Source: podsTargetGroupName,
		Labels: model.LabelSet{
			roleLabel: model.LabelValue("pod"),
		},
	}

	for _, namespace := range kd.pods {
		for _, pod := range namespace {
			tg.Targets = append(tg.Targets, updatePodTargets(pod, false)...)
		}
	}

	return tg
}
