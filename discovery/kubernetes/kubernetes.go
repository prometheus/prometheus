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
	"reflect"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	apiv1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix  = model.MetaLabelPrefix + "kubernetes_"
	namespaceLabel   = metaLabelPrefix + "namespace"
	metricsNamespace = "prometheus_sd_kubernetes"
	presentValue     = model.LabelValue("true")
)

var (
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
	DefaultSDConfig = SDConfig{}
)

// Role is role of the service in Kubernetes.
type Role string

// The valid options for Role.
const (
	RoleNode     Role = "node"
	RolePod      Role = "pod"
	RoleService  Role = "service"
	RoleEndpoint Role = "endpoints"
	RoleIngress  Role = "ingress"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case RoleNode, RolePod, RoleService, RoleEndpoint, RoleIngress:
		return nil
	default:
		return errors.Errorf("unknown Kubernetes SD role %q", *c)
	}
}

// SDConfig is the configuration for Kubernetes service discovery.
type SDConfig struct {
	APIServer          config_util.URL              `yaml:"api_server,omitempty"`
	Role               Role                         `yaml:"role"`
	HTTPClientConfig   config_util.HTTPClientConfig `yaml:",inline"`
	NamespaceDiscovery NamespaceDiscovery           `yaml:"namespaces,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = SDConfig{}
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Role == "" {
		return errors.Errorf("role missing (one of: pod, service, endpoints, node, ingress)")
	}
	err = c.HTTPClientConfig.Validate()
	if err != nil {
		return err
	}
	if c.APIServer.URL == nil && !reflect.DeepEqual(c.HTTPClientConfig, config_util.HTTPClientConfig{}) {
		return errors.Errorf("to use custom HTTP client configuration please provide the 'api_server' URL explicitly")
	}
	return nil
}

// NamespaceDiscovery is the configuration for discovering
// Kubernetes namespaces.
type NamespaceDiscovery struct {
	Names []string `yaml:"names"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *NamespaceDiscovery) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = NamespaceDiscovery{}
	type plain NamespaceDiscovery
	return unmarshal((*plain)(c))
}

func init() {
	prometheus.MustRegister(eventCount)

	// Initialize metric vectors.
	for _, role := range []string{"endpoints", "node", "pod", "service", "ingress"} {
		for _, evt := range []string{"add", "delete", "update"} {
			eventCount.WithLabelValues(role, evt)
		}
	}

	var (
		clientGoRequestMetricAdapterInstance     = clientGoRequestMetricAdapter{}
		clientGoCacheMetricsProviderInstance     = clientGoCacheMetricsProvider{}
		clientGoWorkqueueMetricsProviderInstance = clientGoWorkqueueMetricsProvider{}
	)

	clientGoRequestMetricAdapterInstance.Register(prometheus.DefaultRegisterer)
	clientGoCacheMetricsProviderInstance.Register(prometheus.DefaultRegisterer)
	clientGoWorkqueueMetricsProviderInstance.Register(prometheus.DefaultRegisterer)

}

// This is only for internal use.
type discoverer interface {
	Run(ctx context.Context, up chan<- []*targetgroup.Group)
}

// Discovery implements the discoverer interface for discovering
// targets from Kubernetes.
type Discovery struct {
	sync.RWMutex
	client             kubernetes.Interface
	role               Role
	logger             log.Logger
	namespaceDiscovery *NamespaceDiscovery
	discoverers        []discoverer
}

func (d *Discovery) getNamespaces() []string {
	namespaces := d.namespaceDiscovery.Names
	if len(namespaces) == 0 {
		namespaces = []string{apiv1.NamespaceAll}
	}
	return namespaces
}

// New creates a new Kubernetes discovery for the given role.
func New(l log.Logger, conf *SDConfig) (*Discovery, error) {
	if l == nil {
		l = log.NewNopLogger()
	}
	var (
		kcfg *rest.Config
		err  error
	)
	if conf.APIServer.URL == nil {
		// Use the Kubernetes provided pod service account
		// as described in https://kubernetes.io/docs/admin/service-accounts-admin/
		kcfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		level.Info(l).Log("msg", "Using pod service account via in-cluster config")
	} else {
		rt, err := config_util.NewRoundTripperFromConfig(conf.HTTPClientConfig, "kubernetes_sd")
		if err != nil {
			return nil, err
		}
		kcfg = &rest.Config{
			Host:      conf.APIServer.String(),
			Transport: rt,
		}
	}

	kcfg.UserAgent = "Prometheus/discovery"

	c, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}
	return &Discovery{
		client:             c,
		logger:             l,
		role:               conf.Role,
		namespaceDiscovery: &conf.NamespaceDiscovery,
		discoverers:        make([]discoverer, 0),
	}, nil
}

const resyncPeriod = 10 * time.Minute

// Run implements the discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	d.Lock()
	namespaces := d.getNamespaces()

	switch d.role {
	case RoleEndpoint:
		for _, namespace := range namespaces {
			e := d.client.CoreV1().Endpoints(namespace)
			elw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					return e.List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return e.Watch(options)
				},
			}
			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					return s.List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return s.Watch(options)
				},
			}
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					return p.List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return p.Watch(options)
				},
			}
			eps := NewEndpoints(
				log.With(d.logger, "role", "endpoint"),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
				cache.NewSharedInformer(elw, &apiv1.Endpoints{}, resyncPeriod),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, eps)
			go eps.endpointsInf.Run(ctx.Done())
			go eps.serviceInf.Run(ctx.Done())
			go eps.podInf.Run(ctx.Done())
		}
	case RolePod:
		for _, namespace := range namespaces {
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					return p.List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return p.Watch(options)
				},
			}
			pod := NewPod(
				log.With(d.logger, "role", "pod"),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, pod)
			go pod.informer.Run(ctx.Done())
		}
	case RoleService:
		for _, namespace := range namespaces {
			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					return s.List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return s.Watch(options)
				},
			}
			svc := NewService(
				log.With(d.logger, "role", "service"),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, svc)
			go svc.informer.Run(ctx.Done())
		}
	case RoleIngress:
		for _, namespace := range namespaces {
			i := d.client.ExtensionsV1beta1().Ingresses(namespace)
			ilw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					return i.List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return i.Watch(options)
				},
			}
			ingress := NewIngress(
				log.With(d.logger, "role", "ingress"),
				cache.NewSharedInformer(ilw, &extensionsv1beta1.Ingress{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, ingress)
			go ingress.informer.Run(ctx.Done())
		}
	case RoleNode:
		nlw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return d.client.CoreV1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return d.client.CoreV1().Nodes().Watch(options)
			},
		}
		node := NewNode(
			log.With(d.logger, "role", "node"),
			cache.NewSharedInformer(nlw, &apiv1.Node{}, resyncPeriod),
		)
		d.discoverers = append(d.discoverers, node)
		go node.informer.Run(ctx.Done())
	default:
		level.Error(d.logger).Log("msg", "unknown Kubernetes discovery kind", "role", d.role)
	}

	var wg sync.WaitGroup
	for _, dd := range d.discoverers {
		wg.Add(1)
		go func(d discoverer) {
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

func send(ctx context.Context, l log.Logger, role Role, ch chan<- []*targetgroup.Group, tg *targetgroup.Group) {
	if tg == nil {
		return
	}
	select {
	case <-ctx.Done():
	case ch <- []*targetgroup.Group{tg}:
	}
}
