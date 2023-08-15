// Copyright 2016 The Prometheus Authors
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
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/prometheus/util/strutil"

	disv1beta1 "k8s.io/api/discovery/v1beta1"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	apiv1 "k8s.io/api/core/v1"
	disv1 "k8s.io/api/discovery/v1"
	networkv1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	// Required to get the GCP auth provider working.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	// metaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix  = model.MetaLabelPrefix + "kubernetes_"
	namespaceLabel   = metaLabelPrefix + "namespace"
	metricsNamespace = "prometheus_sd_kubernetes"
	presentValue     = model.LabelValue("true")
)

var (
	// Http header
	userAgent = fmt.Sprintf("Prometheus/%s", version.Version)
	// Custom events metric
	eventCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "events_total",
			Help:      "The number of Kubernetes events handled.",
		},
		[]string{"role", "event"},
	)
	// DefaultSDConfig is the default Kubernetes SD configuration
	DefaultSDConfig = SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
	prometheus.MustRegister(eventCount)
	// Initialize metric vectors.
	for _, role := range []string{"endpointslice", "endpoints", "node", "pod", "service", "ingress"} {
		for _, evt := range []string{"add", "delete", "update"} {
			eventCount.WithLabelValues(role, evt)
		}
	}
	(&clientGoRequestMetricAdapter{}).Register(prometheus.DefaultRegisterer)
	(&clientGoWorkqueueMetricsProvider{}).Register(prometheus.DefaultRegisterer)
}

// Role is role of the service in Kubernetes.
type Role string

// The valid options for Role.
const (
	RoleNode          Role = "node"
	RolePod           Role = "pod"
	RoleService       Role = "service"
	RoleEndpoint      Role = "endpoints"
	RoleEndpointSlice Role = "endpointslice"
	RoleIngress       Role = "ingress"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case RoleNode, RolePod, RoleService, RoleEndpoint, RoleEndpointSlice, RoleIngress:
		return nil
	default:
		return fmt.Errorf("unknown Kubernetes SD role %q", *c)
	}
}

// SDConfig is the configuration for Kubernetes service discovery.
type SDConfig struct {
	APIServer          config.URL              `yaml:"api_server,omitempty"`
	Role               Role                    `yaml:"role"`
	KubeConfig         string                  `yaml:"kubeconfig_file"`
	HTTPClientConfig   config.HTTPClientConfig `yaml:",inline"`
	NamespaceDiscovery NamespaceDiscovery      `yaml:"namespaces,omitempty"`
	Selectors          []SelectorConfig        `yaml:"selectors,omitempty"`
	AttachMetadata     AttachMetadataConfig    `yaml:"attach_metadata,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "kubernetes" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return New(opts.Logger, c)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
	c.KubeConfig = config.JoinDir(dir, c.KubeConfig)
}

type roleSelector struct {
	node          resourceSelector
	pod           resourceSelector
	service       resourceSelector
	endpoints     resourceSelector
	endpointslice resourceSelector
	ingress       resourceSelector
}

type SelectorConfig struct {
	Role  Role   `yaml:"role,omitempty"`
	Label string `yaml:"label,omitempty"`
	Field string `yaml:"field,omitempty"`
}

type resourceSelector struct {
	label string
	field string
}

// AttachMetadataConfig is the configuration for attaching additional metadata
// coming from nodes on which the targets are scheduled.
type AttachMetadataConfig struct {
	Node bool `yaml:"node"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Role == "" {
		return fmt.Errorf("role missing (one of: pod, service, endpoints, endpointslice, node, ingress)")
	}
	err = c.HTTPClientConfig.Validate()
	if err != nil {
		return err
	}
	if c.APIServer.URL != nil && c.KubeConfig != "" {
		// Api-server and kubeconfig_file are mutually exclusive
		return fmt.Errorf("cannot use 'kubeconfig_file' and 'api_server' simultaneously")
	}
	if c.KubeConfig != "" && !reflect.DeepEqual(c.HTTPClientConfig, config.DefaultHTTPClientConfig) {
		// Kubeconfig_file and custom http config are mutually exclusive
		return fmt.Errorf("cannot use a custom HTTP client configuration together with 'kubeconfig_file'")
	}
	if c.APIServer.URL == nil && !reflect.DeepEqual(c.HTTPClientConfig, config.DefaultHTTPClientConfig) {
		return fmt.Errorf("to use custom HTTP client configuration please provide the 'api_server' URL explicitly")
	}
	if c.APIServer.URL != nil && c.NamespaceDiscovery.IncludeOwnNamespace {
		return fmt.Errorf("cannot use 'api_server' and 'namespaces.own_namespace' simultaneously")
	}
	if c.KubeConfig != "" && c.NamespaceDiscovery.IncludeOwnNamespace {
		return fmt.Errorf("cannot use 'kubeconfig_file' and 'namespaces.own_namespace' simultaneously")
	}

	foundSelectorRoles := make(map[Role]struct{})
	allowedSelectors := map[Role][]string{
		RolePod:           {string(RolePod)},
		RoleService:       {string(RoleService)},
		RoleEndpointSlice: {string(RolePod), string(RoleService), string(RoleEndpointSlice)},
		RoleEndpoint:      {string(RolePod), string(RoleService), string(RoleEndpoint)},
		RoleNode:          {string(RoleNode)},
		RoleIngress:       {string(RoleIngress)},
	}

	for _, selector := range c.Selectors {
		if _, ok := foundSelectorRoles[selector.Role]; ok {
			return fmt.Errorf("duplicated selector role: %s", selector.Role)
		}
		foundSelectorRoles[selector.Role] = struct{}{}

		if _, ok := allowedSelectors[c.Role]; !ok {
			return fmt.Errorf("invalid role: %q, expecting one of: pod, service, endpoints, endpointslice, node or ingress", c.Role)
		}
		var allowed bool
		for _, role := range allowedSelectors[c.Role] {
			if role == string(selector.Role) {
				allowed = true
				break
			}
		}

		if !allowed {
			return fmt.Errorf("%s role supports only %s selectors", c.Role, strings.Join(allowedSelectors[c.Role], ", "))
		}

		_, err := fields.ParseSelector(selector.Field)
		if err != nil {
			return err
		}
		_, err = labels.Parse(selector.Label)
		if err != nil {
			return err
		}
	}
	return nil
}

// NamespaceDiscovery is the configuration for discovering
// Kubernetes namespaces.
type NamespaceDiscovery struct {
	IncludeOwnNamespace bool     `yaml:"own_namespace"`
	Names               []string `yaml:"names"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *NamespaceDiscovery) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = NamespaceDiscovery{}
	type plain NamespaceDiscovery
	return unmarshal((*plain)(c))
}

// Discovery implements the discoverer interface for discovering
// targets from Kubernetes.
type Discovery struct {
	sync.RWMutex
	client             kubernetes.Interface
	role               Role
	logger             log.Logger
	namespaceDiscovery *NamespaceDiscovery
	discoverers        []discovery.Discoverer
	selectors          roleSelector
	ownNamespace       string
	attachMetadata     AttachMetadataConfig
}

func (d *Discovery) getNamespaces() []string {
	namespaces := d.namespaceDiscovery.Names
	includeOwnNamespace := d.namespaceDiscovery.IncludeOwnNamespace

	if len(namespaces) == 0 && !includeOwnNamespace {
		return []string{apiv1.NamespaceAll}
	}

	if includeOwnNamespace && d.ownNamespace != "" {
		return append(namespaces, d.ownNamespace)
	}

	return namespaces
}

// New creates a new Kubernetes discovery for the given role.
func New(l log.Logger, conf *SDConfig) (*Discovery, error) {
	if l == nil {
		l = log.NewNopLogger()
	}
	var (
		kcfg         *rest.Config
		err          error
		ownNamespace string
	)
	switch {
	case conf.KubeConfig != "":
		kcfg, err = clientcmd.BuildConfigFromFlags("", conf.KubeConfig)
		if err != nil {
			return nil, err
		}
	case conf.APIServer.URL == nil:
		// Use the Kubernetes provided pod service account
		// as described in https://kubernetes.io/docs/admin/service-accounts-admin/
		kcfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}

		if conf.NamespaceDiscovery.IncludeOwnNamespace {
			ownNamespaceContents, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
			if err != nil {
				return nil, fmt.Errorf("could not determine the pod's namespace: %w", err)
			}
			if len(ownNamespaceContents) == 0 {
				return nil, errors.New("could not read own namespace name (empty file)")
			}
			ownNamespace = string(ownNamespaceContents)
		}

		level.Info(l).Log("msg", "Using pod service account via in-cluster config")
	default:
		rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "kubernetes_sd")
		if err != nil {
			return nil, err
		}
		kcfg = &rest.Config{
			Host:      conf.APIServer.String(),
			Transport: rt,
		}
	}

	kcfg.UserAgent = userAgent
	kcfg.ContentType = "application/vnd.kubernetes.protobuf"

	c, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}

	return &Discovery{
		client:             c,
		logger:             l,
		role:               conf.Role,
		namespaceDiscovery: &conf.NamespaceDiscovery,
		discoverers:        make([]discovery.Discoverer, 0),
		selectors:          mapSelector(conf.Selectors),
		ownNamespace:       ownNamespace,
		attachMetadata:     conf.AttachMetadata,
	}, nil
}

func mapSelector(rawSelector []SelectorConfig) roleSelector {
	rs := roleSelector{}
	for _, resourceSelectorRaw := range rawSelector {
		switch resourceSelectorRaw.Role {
		case RoleEndpointSlice:
			rs.endpointslice.field = resourceSelectorRaw.Field
			rs.endpointslice.label = resourceSelectorRaw.Label
		case RoleEndpoint:
			rs.endpoints.field = resourceSelectorRaw.Field
			rs.endpoints.label = resourceSelectorRaw.Label
		case RoleIngress:
			rs.ingress.field = resourceSelectorRaw.Field
			rs.ingress.label = resourceSelectorRaw.Label
		case RoleNode:
			rs.node.field = resourceSelectorRaw.Field
			rs.node.label = resourceSelectorRaw.Label
		case RolePod:
			rs.pod.field = resourceSelectorRaw.Field
			rs.pod.label = resourceSelectorRaw.Label
		case RoleService:
			rs.service.field = resourceSelectorRaw.Field
			rs.service.label = resourceSelectorRaw.Label
		}
	}
	return rs
}

// Disable the informer's resync, which just periodically resends already processed updates and distort SD metrics.
const resyncDisabled = 0

// Run implements the discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	d.Lock()
	namespaces := d.getNamespaces()

	switch d.role {
	case RoleEndpointSlice:
		// Check "networking.k8s.io/v1" availability with retries.
		// If "v1" is not available, use "networking.k8s.io/v1beta1" for backward compatibility
		var v1Supported bool
		if retryOnError(ctx, 10*time.Second,
			func() (err error) {
				v1Supported, err = checkDiscoveryV1Supported(d.client)
				if err != nil {
					level.Error(d.logger).Log("msg", "Failed to check networking.k8s.io/v1 availability", "err", err)
				}
				return err
			},
		) {
			d.Unlock()
			return
		}

		for _, namespace := range namespaces {
			var informer cache.SharedIndexInformer
			if v1Supported {
				e := d.client.DiscoveryV1().EndpointSlices(namespace)
				elw := &cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
						options.FieldSelector = d.selectors.endpointslice.field
						options.LabelSelector = d.selectors.endpointslice.label
						return e.List(ctx, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						options.FieldSelector = d.selectors.endpointslice.field
						options.LabelSelector = d.selectors.endpointslice.label
						return e.Watch(ctx, options)
					},
				}
				informer = d.newEndpointSlicesByNodeInformer(elw, &disv1.EndpointSlice{})
			} else {
				e := d.client.DiscoveryV1beta1().EndpointSlices(namespace)
				elw := &cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
						options.FieldSelector = d.selectors.endpointslice.field
						options.LabelSelector = d.selectors.endpointslice.label
						return e.List(ctx, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						options.FieldSelector = d.selectors.endpointslice.field
						options.LabelSelector = d.selectors.endpointslice.label
						return e.Watch(ctx, options)
					},
				}
				informer = d.newEndpointSlicesByNodeInformer(elw, &disv1beta1.EndpointSlice{})
			}

			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.Watch(ctx, options)
				},
			}
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.Watch(ctx, options)
				},
			}
			var nodeInf cache.SharedInformer
			if d.attachMetadata.Node {
				nodeInf = d.newNodeInformer(context.Background())
				go nodeInf.Run(ctx.Done())
			}
			eps := NewEndpointSlice(
				log.With(d.logger, "role", "endpointslice"),
				informer,
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncDisabled),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncDisabled),
				nodeInf,
			)
			d.discoverers = append(d.discoverers, eps)
			go eps.endpointSliceInf.Run(ctx.Done())
			go eps.serviceInf.Run(ctx.Done())
			go eps.podInf.Run(ctx.Done())
		}
	case RoleEndpoint:
		for _, namespace := range namespaces {
			e := d.client.CoreV1().Endpoints(namespace)
			elw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.endpoints.field
					options.LabelSelector = d.selectors.endpoints.label
					return e.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.endpoints.field
					options.LabelSelector = d.selectors.endpoints.label
					return e.Watch(ctx, options)
				},
			}
			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.Watch(ctx, options)
				},
			}
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.Watch(ctx, options)
				},
			}
			var nodeInf cache.SharedInformer
			if d.attachMetadata.Node {
				nodeInf = d.newNodeInformer(ctx)
				go nodeInf.Run(ctx.Done())
			}

			eps := NewEndpoints(
				log.With(d.logger, "role", "endpoint"),
				d.newEndpointsByNodeInformer(elw),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncDisabled),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncDisabled),
				nodeInf,
			)
			d.discoverers = append(d.discoverers, eps)
			go eps.endpointsInf.Run(ctx.Done())
			go eps.serviceInf.Run(ctx.Done())
			go eps.podInf.Run(ctx.Done())
		}
	case RolePod:
		var nodeInformer cache.SharedInformer
		if d.attachMetadata.Node {
			nodeInformer = d.newNodeInformer(ctx)
			go nodeInformer.Run(ctx.Done())
		}

		for _, namespace := range namespaces {
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.Watch(ctx, options)
				},
			}
			pod := NewPod(
				log.With(d.logger, "role", "pod"),
				d.newPodsByNodeInformer(plw),
				nodeInformer,
			)
			d.discoverers = append(d.discoverers, pod)
			go pod.podInf.Run(ctx.Done())
		}
	case RoleService:
		for _, namespace := range namespaces {
			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.Watch(ctx, options)
				},
			}
			svc := NewService(
				log.With(d.logger, "role", "service"),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncDisabled),
			)
			d.discoverers = append(d.discoverers, svc)
			go svc.informer.Run(ctx.Done())
		}
	case RoleIngress:
		// Check "networking.k8s.io/v1" availability with retries.
		// If "v1" is not available, use "networking.k8s.io/v1beta1" for backward compatibility
		var v1Supported bool
		if retryOnError(ctx, 10*time.Second,
			func() (err error) {
				v1Supported, err = checkNetworkingV1Supported(d.client)
				if err != nil {
					level.Error(d.logger).Log("msg", "Failed to check networking.k8s.io/v1 availability", "err", err)
				}
				return err
			},
		) {
			d.Unlock()
			return
		}

		for _, namespace := range namespaces {
			var informer cache.SharedInformer
			if v1Supported {
				i := d.client.NetworkingV1().Ingresses(namespace)
				ilw := &cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.List(ctx, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.Watch(ctx, options)
					},
				}
				informer = cache.NewSharedInformer(ilw, &networkv1.Ingress{}, resyncDisabled)
			} else {
				i := d.client.NetworkingV1beta1().Ingresses(namespace)
				ilw := &cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.List(ctx, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.Watch(ctx, options)
					},
				}
				informer = cache.NewSharedInformer(ilw, &v1beta1.Ingress{}, resyncDisabled)
			}
			ingress := NewIngress(
				log.With(d.logger, "role", "ingress"),
				informer,
			)
			d.discoverers = append(d.discoverers, ingress)
			go ingress.informer.Run(ctx.Done())
		}
	case RoleNode:
		nodeInformer := d.newNodeInformer(ctx)
		node := NewNode(log.With(d.logger, "role", "node"), nodeInformer)
		d.discoverers = append(d.discoverers, node)
		go node.informer.Run(ctx.Done())
	default:
		level.Error(d.logger).Log("msg", "unknown Kubernetes discovery kind", "role", d.role)
	}

	var wg sync.WaitGroup
	for _, dd := range d.discoverers {
		wg.Add(1)
		go func(d discovery.Discoverer) {
			defer wg.Done()
			d.Run(ctx, ch)
		}(dd)
	}

	d.Unlock()

	wg.Wait()
	<-ctx.Done()
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}

func send(ctx context.Context, ch chan<- []*targetgroup.Group, tg *targetgroup.Group) {
	if tg == nil {
		return
	}
	select {
	case <-ctx.Done():
	case ch <- []*targetgroup.Group{tg}:
	}
}

func retryOnError(ctx context.Context, interval time.Duration, f func() error) (canceled bool) {
	var err error
	err = f()
	for {
		if err == nil {
			return false
		}
		select {
		case <-ctx.Done():
			return true
		case <-time.After(interval):
			err = f()
		}
	}
}

func checkNetworkingV1Supported(client kubernetes.Interface) (bool, error) {
	k8sVer, err := client.Discovery().ServerVersion()
	if err != nil {
		return false, err
	}
	semVer, err := utilversion.ParseSemantic(k8sVer.String())
	if err != nil {
		return false, err
	}
	// networking.k8s.io/v1 is available since Kubernetes v1.19
	// https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.19.md
	return semVer.Major() >= 1 && semVer.Minor() >= 19, nil
}

func (d *Discovery) newNodeInformer(ctx context.Context) cache.SharedInformer {
	nlw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = d.selectors.node.field
			options.LabelSelector = d.selectors.node.label
			return d.client.CoreV1().Nodes().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = d.selectors.node.field
			options.LabelSelector = d.selectors.node.label
			return d.client.CoreV1().Nodes().Watch(ctx, options)
		},
	}
	return cache.NewSharedInformer(nlw, &apiv1.Node{}, resyncDisabled)
}

func (d *Discovery) newPodsByNodeInformer(plw *cache.ListWatch) cache.SharedIndexInformer {
	indexers := make(map[string]cache.IndexFunc)
	if d.attachMetadata.Node {
		indexers[nodeIndex] = func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*apiv1.Pod)
			if !ok {
				return nil, fmt.Errorf("object is not a pod")
			}
			return []string{pod.Spec.NodeName}, nil
		}
	}

	return cache.NewSharedIndexInformer(plw, &apiv1.Pod{}, resyncDisabled, indexers)
}

func (d *Discovery) newEndpointsByNodeInformer(plw *cache.ListWatch) cache.SharedIndexInformer {
	indexers := make(map[string]cache.IndexFunc)
	if !d.attachMetadata.Node {
		return cache.NewSharedIndexInformer(plw, &apiv1.Endpoints{}, resyncDisabled, indexers)
	}

	indexers[nodeIndex] = func(obj interface{}) ([]string, error) {
		e, ok := obj.(*apiv1.Endpoints)
		if !ok {
			return nil, fmt.Errorf("object is not endpoints")
		}
		var nodes []string
		for _, target := range e.Subsets {
			for _, addr := range target.Addresses {
				if addr.TargetRef != nil {
					switch addr.TargetRef.Kind {
					case "Pod":
						if addr.NodeName != nil {
							nodes = append(nodes, *addr.NodeName)
						}
					case "Node":
						nodes = append(nodes, addr.TargetRef.Name)
					}
				}
			}
		}
		return nodes, nil
	}

	return cache.NewSharedIndexInformer(plw, &apiv1.Endpoints{}, resyncDisabled, indexers)
}

func (d *Discovery) newEndpointSlicesByNodeInformer(plw *cache.ListWatch, object runtime.Object) cache.SharedIndexInformer {
	indexers := make(map[string]cache.IndexFunc)
	if !d.attachMetadata.Node {
		cache.NewSharedIndexInformer(plw, &disv1.EndpointSlice{}, resyncDisabled, indexers)
	}

	indexers[nodeIndex] = func(obj interface{}) ([]string, error) {
		var nodes []string
		switch e := obj.(type) {
		case *disv1.EndpointSlice:
			for _, target := range e.Endpoints {
				if target.TargetRef != nil {
					switch target.TargetRef.Kind {
					case "Pod":
						if target.NodeName != nil {
							nodes = append(nodes, *target.NodeName)
						}
					case "Node":
						nodes = append(nodes, target.TargetRef.Name)
					}
				}
			}
		case *disv1beta1.EndpointSlice:
			for _, target := range e.Endpoints {
				if target.TargetRef != nil {
					switch target.TargetRef.Kind {
					case "Pod":
						if target.NodeName != nil {
							nodes = append(nodes, *target.NodeName)
						}
					case "Node":
						nodes = append(nodes, target.TargetRef.Name)
					}
				}
			}
		default:
			return nil, fmt.Errorf("object is not an endpointslice")
		}

		return nodes, nil
	}

	return cache.NewSharedIndexInformer(plw, object, resyncDisabled, indexers)
}

func checkDiscoveryV1Supported(client kubernetes.Interface) (bool, error) {
	k8sVer, err := client.Discovery().ServerVersion()
	if err != nil {
		return false, err
	}
	semVer, err := utilversion.ParseSemantic(k8sVer.String())
	if err != nil {
		return false, err
	}
	// The discovery.k8s.io/v1beta1 API version of EndpointSlice will no longer be served in v1.25.
	// discovery.k8s.io/v1 is available since Kubernetes v1.21
	// https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-25
	return semVer.Major() >= 1 && semVer.Minor() >= 21, nil
}

func addObjectMetaLabels(labelSet model.LabelSet, objectMeta metav1.ObjectMeta, role Role) {
	labelSet[model.LabelName(metaLabelPrefix+string(role)+"_name")] = lv(objectMeta.Name)

	for k, v := range objectMeta.Labels {
		ln := strutil.SanitizeLabelName(k)
		labelSet[model.LabelName(metaLabelPrefix+string(role)+"_label_"+ln)] = lv(v)
		labelSet[model.LabelName(metaLabelPrefix+string(role)+"_labelpresent_"+ln)] = presentValue
	}

	for k, v := range objectMeta.Annotations {
		ln := strutil.SanitizeLabelName(k)
		labelSet[model.LabelName(metaLabelPrefix+string(role)+"_annotation_"+ln)] = lv(v)
		labelSet[model.LabelName(metaLabelPrefix+string(role)+"_annotationpresent_"+ln)] = presentValue
	}
}
