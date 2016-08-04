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
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
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
	// podNodeLabel is the name for the label containing the name of the node that a pod is scheduled on to
	podNodeNameLabel = metaLabelPrefix + "pod_node_name"
	// podHostIPLabel is the name for the label containing the IP of the node that a pod is scheduled on to
	podHostIPLabel = metaLabelPrefix + "pod_host_ip"

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
	// nodeAddressPrefix is the prefix for the node addresses.
	nodeAddressPrefix = metaLabelPrefix + "node_address_"
	// nodePortLabel is the name of the label for the node port.
	nodePortLabel = metaLabelPrefix + "node_port"

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
}

// Initialize sets up the discovery for usage.
func (kd *Discovery) Initialize() error {
	client, err := newKubernetesHTTPClient(kd.Conf)

	if err != nil {
		return err
	}

	kd.apiServers = kd.Conf.APIServers
	kd.client = client

	return nil
}

// Run implements the TargetProvider interface.
func (kd *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	log.Debugf("Start Kubernetes service discovery")
	defer close(ch)

	switch kd.Conf.Role {
	case config.KubernetesRolePod, config.KubernetesRoleContainer:
		pd := &podDiscovery{
			retryInterval: time.Duration(kd.Conf.RetryInterval),
			kd:            kd,
		}
		pd.run(ctx, ch)
	case config.KubernetesRoleNode:
		nd := &nodeDiscovery{
			retryInterval: time.Duration(kd.Conf.RetryInterval),
			kd:            kd,
		}
		nd.run(ctx, ch)
	case config.KubernetesRoleService, config.KubernetesRoleEndpoint:
		sd := &serviceDiscovery{
			retryInterval: time.Duration(kd.Conf.RetryInterval),
			kd:            kd,
		}
		sd.run(ctx, ch)
	case config.KubernetesRoleAPIServer:
		select {
		case ch <- []*config.TargetGroup{kd.updateAPIServersTargetGroup()}:
		case <-ctx.Done():
			return
		}
	default:
		log.Errorf("unknown Kubernetes discovery kind %q", kd.Conf.Role)
		return
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
