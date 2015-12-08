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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	sourceServicePrefix = "services"

	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "kubernetes_"
	// serviceNamespaceLabel is the name for the label containing a target's service namespace.
	serviceNamespaceLabel = metaLabelPrefix + "service_namespace"
	// serviceNameLabel is the name for the label containing a target's service name.
	serviceNameLabel = metaLabelPrefix + "service_name"
	// nodeLabelPrefix is the prefix for the node labels.
	nodeLabelPrefix = metaLabelPrefix + "node_label_"
	// serviceLabelPrefix is the prefix for the service labels.
	serviceLabelPrefix = metaLabelPrefix + "service_label_"
	// serviceAnnotationPrefix is the prefix for the service annotations.
	serviceAnnotationPrefix = metaLabelPrefix + "service_annotation_"
	// nodesTargetGroupName is the name given to the target group for nodes.
	nodesTargetGroupName = "nodes"
	// apiServersTargetGroupName is the name given to the target group for API servers.
	apiServersTargetGroupName = "apiServers"
	// roleLabel is the name for the label containing a target's role.
	roleLabel = metaLabelPrefix + "role"

	serviceAccountToken  = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	serviceAccountCACert = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

	apiVersion          = "v1"
	apiPrefix           = "/api/" + apiVersion
	nodesURL            = apiPrefix + "/nodes"
	servicesURL         = apiPrefix + "/services"
	endpointsURL        = apiPrefix + "/endpoints"
	serviceEndpointsURL = apiPrefix + "/namespaces/%s/endpoints/%s"
)

// Discovery implements a TargetProvider for Kubernetes services.
type Discovery struct {
	client *http.Client
	Conf   *config.KubernetesSDConfig

	apiServers               []config.URL
	apiServersMu             sync.RWMutex
	nodesResourceVersion     string
	servicesResourceVersion  string
	endpointsResourceVersion string
	nodes                    map[string]*Node
	services                 map[string]map[string]*Service
	nodesMu                  sync.RWMutex
	servicesMu               sync.RWMutex
	runDone                  chan struct{}
}

// Initialize sets up the discovery for usage.
func (kd *Discovery) Initialize() error {
	client, err := newKubernetesHTTPClient(kd.Conf)

	if err != nil {
		return err
	}

	kd.apiServers = kd.Conf.APIServers
	kd.client = client
	kd.nodes = map[string]*Node{}
	kd.services = map[string]map[string]*Service{}
	kd.runDone = make(chan struct{})

	return nil
}

// Sources implements the TargetProvider interface.
func (kd *Discovery) Sources() []string {
	sourceNames := make([]string, 0, len(kd.apiServers))
	for _, apiServer := range kd.apiServers {
		sourceNames = append(sourceNames, apiServersTargetGroupName+":"+apiServer.Host)
	}

	res, err := kd.queryAPIServerPath(nodesURL)
	if err != nil {
		// If we can't list nodes then we can't watch them. Assume this is a misconfiguration
		// & log & return empty.
		log.Errorf("Unable to list Kubernetes nodes: %s", err)
		return []string{}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Unable to list Kubernetes nodes. Unexpected response: %d %s", res.StatusCode, res.Status)
		return []string{}
	}

	var nodes NodeList
	if err := json.NewDecoder(res.Body).Decode(&nodes); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		log.Errorf("Unable to list Kubernetes nodes. Unexpected response body: %s", string(body))
		return []string{}
	}

	kd.nodesMu.Lock()
	defer kd.nodesMu.Unlock()

	kd.nodesResourceVersion = nodes.ResourceVersion
	for idx, node := range nodes.Items {
		sourceNames = append(sourceNames, nodesTargetGroupName+":"+node.ObjectMeta.Name)
		kd.nodes[node.ObjectMeta.Name] = &nodes.Items[idx]
	}

	res, err = kd.queryAPIServerPath(servicesURL)
	if err != nil {
		// If we can't list services then we can't watch them. Assume this is a misconfiguration
		// & log & return empty.
		log.Errorf("Unable to list Kubernetes services: %s", err)
		return []string{}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Unable to list Kubernetes services. Unexpected response: %d %s", res.StatusCode, res.Status)
		return []string{}
	}
	var services ServiceList
	if err := json.NewDecoder(res.Body).Decode(&services); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		log.Errorf("Unable to list Kubernetes services. Unexpected response body: %s", string(body))
		return []string{}
	}
	kd.servicesMu.Lock()
	defer kd.servicesMu.Unlock()

	kd.servicesResourceVersion = services.ResourceVersion
	for idx, service := range services.Items {
		sourceNames = append(sourceNames, serviceSource(&service))
		namespace, ok := kd.services[service.ObjectMeta.Namespace]
		if !ok {
			namespace = map[string]*Service{}
			kd.services[service.ObjectMeta.Namespace] = namespace
		}
		namespace[service.ObjectMeta.Name] = &services.Items[idx]
	}

	return sourceNames
}

// Run implements the TargetProvider interface.
func (kd *Discovery) Run(ch chan<- config.TargetGroup, done <-chan struct{}) {
	defer close(ch)

	if tg := kd.updateAPIServersTargetGroup(); tg != nil {
		select {
		case ch <- *tg:
		case <-done:
			return
		}
	}

	if tg := kd.updateNodesTargetGroup(); tg != nil {
		select {
		case ch <- *tg:
		case <-done:
			return
		}
	}

	for _, ns := range kd.services {
		for _, service := range ns {
			tg := kd.addService(service)

			if tg == nil {
				continue
			}

			select {
			case ch <- *tg:
			case <-done:
				return
			}
		}
	}

	retryInterval := time.Duration(kd.Conf.RetryInterval)

	update := make(chan interface{}, 10)

	go kd.watchNodes(update, done, retryInterval)
	go kd.watchServices(update, done, retryInterval)
	go kd.watchServiceEndpoints(update, done, retryInterval)

	var tg *config.TargetGroup
	for {
		select {
		case <-done:
			return
		case event := <-update:
			switch obj := event.(type) {
			case *nodeEvent:
				kd.updateNode(obj.Node, obj.EventType)
				tg = kd.updateNodesTargetGroup()
			case *serviceEvent:
				tg = kd.updateService(obj.Service, obj.EventType)
			case *endpointsEvent:
				tg = kd.updateServiceEndpoints(obj.Endpoints, obj.EventType)
			}
		}

		if tg == nil {
			continue
		}

		select {
		case ch <- *tg:
		case <-done:
			return
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
	return nil, fmt.Errorf("Unable to query any API servers: %v", lastErr)
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
	kd.nodesMu.Lock()
	defer kd.nodesMu.Unlock()

	tg := &config.TargetGroup{
		Source: nodesTargetGroupName,
		Labels: model.LabelSet{
			roleLabel: model.LabelValue("node"),
		},
	}

	// Now let's loop through the nodes & add them to the target group with appropriate labels.
	for nodeName, node := range kd.nodes {
		address := fmt.Sprintf("%s:%d", node.Status.Addresses[0].Address, kd.Conf.KubeletPort)

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
	case deleted:
		// Deleted - remove from nodes map.
		delete(kd.nodes, updatedNodeName)
	case added, modified:
		// Added/Modified - update the node in the nodes map.
		kd.nodes[updatedNodeName] = node
	}
}

// watchNodes watches nodes as they come & go.
func (kd *Discovery) watchNodes(events chan interface{}, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		req, err := http.NewRequest("GET", nodesURL, nil)
		if err != nil {
			log.Errorf("Failed to watch nodes: %s", err)
			return
		}
		values := req.URL.Query()
		values.Add("watch", "true")
		values.Add("resourceVersion", kd.nodesResourceVersion)
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
				log.Errorf("Failed to watch nodes: %s", err)
				return
			}
			kd.nodesResourceVersion = event.Node.ObjectMeta.ResourceVersion

			select {
			case events <- &event:
			case <-done:
			}
		}
	}, retryInterval, done)
}

// watchServices watches services as they come & go.
func (kd *Discovery) watchServices(events chan interface{}, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		req, err := http.NewRequest("GET", servicesURL, nil)
		if err != nil {
			log.Errorf("Failed to watch services: %s", err)
			return
		}
		values := req.URL.Query()
		values.Add("watch", "true")
		values.Add("resourceVersion", kd.servicesResourceVersion)
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
				log.Errorf("Unable to watch services: %s", err)
				return
			}
			kd.servicesResourceVersion = event.Service.ObjectMeta.ResourceVersion

			select {
			case events <- &event:
			case <-done:
			}
		}
	}, retryInterval, done)
}

func (kd *Discovery) updateService(service *Service, eventType EventType) *config.TargetGroup {
	kd.servicesMu.Lock()
	defer kd.servicesMu.Unlock()

	var (
		name      = service.ObjectMeta.Name
		namespace = service.ObjectMeta.Namespace
		_, exists = kd.services[namespace][name]
	)

	switch eventType {
	case deleted:
		if exists {
			return kd.deleteService(service)
		}
	case added, modified:
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

// watchServiceEndpoints watches service endpoints as they come & go.
func (kd *Discovery) watchServiceEndpoints(events chan interface{}, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		req, err := http.NewRequest("GET", endpointsURL, nil)
		if err != nil {
			log.Errorf("Failed to watch service endpoints: %s", err)
			return
		}
		values := req.URL.Query()
		values.Add("watch", "true")
		values.Add("resourceVersion", kd.servicesResourceVersion)
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
				log.Errorf("Unable to watch service endpoints: %s", err)
				return
			}
			kd.servicesResourceVersion = event.Endpoints.ObjectMeta.ResourceVersion

			select {
			case events <- &event:
			case <-done:
			}
		}
	}, retryInterval, done)
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
