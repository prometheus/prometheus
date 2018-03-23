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
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	yaml_util "github.com/prometheus/prometheus/util/yaml"
	yaml "gopkg.in/yaml.v2"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/rest"
)

const (
	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "kubernetes_"
	namespaceLabel  = metaLabelPrefix + "namespace"
)

var (
	eventCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_kubernetes_events_total",
			Help: "The number of Kubernetes events handled.",
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
		return fmt.Errorf("Unknown Kubernetes SD role %q", *c)
	}
}

// SDConfig is the configuration for Kubernetes service discovery.
type SDConfig struct {
	APIServer          config_util.URL        `yaml:"api_server"`
	Role               Role                   `yaml:"role"`
	BasicAuth          *config_util.BasicAuth `yaml:"basic_auth,omitempty"`
	BearerToken        config_util.Secret     `yaml:"bearer_token,omitempty"`
	BearerTokenFile    string                 `yaml:"bearer_token_file,omitempty"`
	TLSConfig          config_util.TLSConfig  `yaml:"tls_config,omitempty"`
	NamespaceDiscovery NamespaceDiscovery     `yaml:"namespaces"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

func (c *SDConfig) uniqueKeyForShare() (string, error) {
	cfg := *c
	// unset unnecessary fields
	cfg.Role = ""
	cfg.NamespaceDiscovery = NamespaceDiscovery{}
	cfg.XXX = nil
	byt, err := yaml.Marshal(cfg)
	return string(byt), err
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = SDConfig{}
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := yaml_util.CheckOverflow(c.XXX, "kubernetes_sd_config"); err != nil {
		return err
	}
	if c.Role == "" {
		return fmt.Errorf("role missing (one of: pod, service, endpoints, node)")
	}
	if len(c.BearerToken) > 0 && len(c.BearerTokenFile) > 0 {
		return fmt.Errorf("at most one of bearer_token & bearer_token_file must be configured")
	}
	if c.BasicAuth != nil && (len(c.BearerToken) > 0 || len(c.BearerTokenFile) > 0) {
		return fmt.Errorf("at most one of basic_auth, bearer_token & bearer_token_file must be configured")
	}
	if c.APIServer.URL == nil &&
		(c.BasicAuth != nil || c.BearerToken != "" || c.BearerTokenFile != "" ||
			c.TLSConfig.CAFile != "" || c.TLSConfig.CertFile != "" || c.TLSConfig.KeyFile != "") {
		return fmt.Errorf("to use custom authentication please provide the 'api_server' URL explicitly")
	}
	return nil
}

// NamespaceDiscovery is the configuration for discovering
// Kubernetes namespaces.
type NamespaceDiscovery struct {
	Names []string `yaml:"names"`
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *NamespaceDiscovery) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = NamespaceDiscovery{}
	type plain NamespaceDiscovery
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return yaml_util.CheckOverflow(c.XXX, "namespaces")
}

func init() {
	prometheus.MustRegister(eventCount)

	// Initialize metric vectors.
	for _, role := range []string{"endpoints", "node", "pod", "service"} {
		for _, evt := range []string{"add", "delete", "update"} {
			eventCount.WithLabelValues(role, evt)
		}
	}
}

// Discovery implements the Discoverer interface for discovering
// targets from Kubernetes.
type Discovery struct {
	kubeShared         KubernetesShared
	role               Role
	logger             log.Logger
	namespaceDiscovery *NamespaceDiscovery
}

func (d *Discovery) getNamespaces() []string {
	namespaces := d.namespaceDiscovery.Names
	if len(namespaces) == 0 {
		namespaces = []string{api.NamespaceAll}
	}
	return namespaces
}

func clientFromConfig(l log.Logger, conf *SDConfig) (kubernetes.Interface, error) {
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
		// Because the handling of configuration parameters changes
		// we should inform the user when their currently configured values
		// will be ignored due to precedence of InClusterConfig
		level.Info(l).Log("msg", "Using pod service account via in-cluster config")

		if conf.TLSConfig.CAFile != "" {
			level.Warn(l).Log("msg", "Configured TLS CA file is ignored when using pod service account")
		}
		if conf.TLSConfig.CertFile != "" || conf.TLSConfig.KeyFile != "" {
			level.Warn(l).Log("msg", "Configured TLS client certificate is ignored when using pod service account")
		}
		if conf.BearerToken != "" {
			level.Warn(l).Log("msg", "Configured auth token is ignored when using pod service account")
		}
		if conf.BasicAuth != nil {
			level.Warn(l).Log("msg", "Configured basic authentication credentials are ignored when using pod service account")
		}
	} else {
		kcfg = &rest.Config{
			Host: conf.APIServer.String(),
			TLSClientConfig: rest.TLSClientConfig{
				CAFile:   conf.TLSConfig.CAFile,
				CertFile: conf.TLSConfig.CertFile,
				KeyFile:  conf.TLSConfig.KeyFile,
				Insecure: conf.TLSConfig.InsecureSkipVerify,
			},
		}
		token := string(conf.BearerToken)
		if conf.BearerTokenFile != "" {
			bf, err := ioutil.ReadFile(conf.BearerTokenFile)
			if err != nil {
				return nil, err
			}
			token = string(bf)
		}
		kcfg.BearerToken = token

		if conf.BasicAuth != nil {
			kcfg.Username = conf.BasicAuth.Username
			kcfg.Password = string(conf.BasicAuth.Password)
		}
	}

	// Enable timeout to prevent hanging in case of network issues.
	kcfg.Timeout = clientTimeout
	// Disable throttling to reduce latency in large cluster.
	kcfg.QPS = float32(-1)
	kcfg.UserAgent = "prometheus/discovery"

	return kubernetes.NewForConfig(kcfg)
}

// New creates a new Kubernetes discovery for the given role.
func New(kubeSharedCache KubernetesSharedCache, l log.Logger, conf *SDConfig) (*Discovery, error) {
	if l == nil {
		l = log.NewNopLogger()
	}
	key, err := conf.uniqueKeyForShare()
	if err != nil {
		return nil, err
	}
	shared, err := kubeSharedCache.GetOrCreate(key, func() (*kubernetesShared, error) {
		client, err := clientFromConfig(l, conf)
		if err != nil {
			return nil, err
		}
		return newKubernetesShared(client), nil
	})
	if err != nil {
		return nil, err
	}
	return &Discovery{
		kubeShared:         shared,
		logger:             l,
		role:               conf.Role,
		namespaceDiscovery: &conf.NamespaceDiscovery,
	}, nil
}

const resyncPeriod = 10 * time.Minute
const clientTimeout = 10 * time.Minute

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	namespaces := d.getNamespaces()

	var wg sync.WaitGroup
	switch d.role {
	case "endpoints":
		for _, namespace := range namespaces {
			eps := NewEndpoints(
				log.With(d.logger, "role", "endpoint"),
				d.kubeShared.MustGetSharedInformer("services", namespace),
				d.kubeShared.MustGetSharedInformer("endpoints", namespace),
				d.kubeShared.MustGetSharedInformer("pods", namespace),
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				eps.Run(ctx, ch)
			}()
		}
	case "pod":
		for _, namespace := range namespaces {
			pod := NewPod(
				log.With(d.logger, "role", "pod"),
				d.kubeShared.MustGetSharedInformer("pods", namespace),
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				pod.Run(ctx, ch)
			}()
		}
	case "service":
		for _, namespace := range namespaces {
			svc := NewService(
				log.With(d.logger, "role", "service"),
				d.kubeShared.MustGetSharedInformer("services", namespace),
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				svc.Run(ctx, ch)
			}()
		}
	case "ingress":
		for _, namespace := range namespaces {
			ingress := NewIngress(
				log.With(d.logger, "role", "ingress"),
				d.kubeShared.MustGetSharedInformer("ingresses", namespace),
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				ingress.Run(ctx, ch)
			}()
		}
	case "node":
		node := NewNode(
			log.With(d.logger, "role", "node"),
			d.kubeShared.MustGetSharedInformer("nodes", api.NamespaceAll),
		)
		node.Run(ctx, ch)

	default:
		level.Error(d.logger).Log("msg", "unknown Kubernetes discovery kind", "role", d.role)
	}

	namespacesStr := strings.Join(namespaces, ",")
	if d.role == "node" {
		namespacesStr = api.NamespaceAll
	}
	level.Info(d.logger).Log("msg", "Discoverying targets", "role", d.role, "namespaces", namespacesStr)

	wg.Wait()
	<-ctx.Done()
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}
