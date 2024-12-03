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

package marathon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	// metaLabelPrefix is the meta prefix used for all meta labels in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "marathon_"
	// appLabelPrefix is the prefix for the application labels.
	appLabelPrefix = metaLabelPrefix + "app_label_"

	// appLabel is used for the name of the app in Marathon.
	appLabel model.LabelName = metaLabelPrefix + "app"
	// imageLabel is the label that is used for the docker image running the service.
	imageLabel model.LabelName = metaLabelPrefix + "image"
	// portIndexLabel is the integer port index when multiple ports are defined;
	// e.g. PORT1 would have a value of '1'.
	portIndexLabel model.LabelName = metaLabelPrefix + "port_index"
	// taskLabel contains the mesos task name of the app instance.
	taskLabel model.LabelName = metaLabelPrefix + "task"

	// portMappingLabelPrefix is the prefix for the application portMappings labels.
	portMappingLabelPrefix = metaLabelPrefix + "port_mapping_label_"
	// portDefinitionLabelPrefix is the prefix for the application portDefinitions labels.
	portDefinitionLabelPrefix = metaLabelPrefix + "port_definition_label_"
)

// DefaultSDConfig is the default Marathon SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval:  model.Duration(30 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for services running on Marathon.
type SDConfig struct {
	Servers          []string                `yaml:"servers,omitempty"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	AuthToken        config.Secret           `yaml:"auth_token,omitempty"`
	AuthTokenFile    string                  `yaml:"auth_token_file,omitempty"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &marathonMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "marathon" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(*c, opts.Logger, opts.Metrics)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
	c.AuthTokenFile = config.JoinDir(dir, c.AuthTokenFile)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if len(c.Servers) == 0 {
		return errors.New("marathon_sd: must contain at least one Marathon server")
	}
	if len(c.AuthToken) > 0 && len(c.AuthTokenFile) > 0 {
		return errors.New("marathon_sd: at most one of auth_token & auth_token_file must be configured")
	}

	if len(c.AuthToken) > 0 || len(c.AuthTokenFile) > 0 {
		switch {
		case c.HTTPClientConfig.BasicAuth != nil:
			return errors.New("marathon_sd: at most one of basic_auth, auth_token & auth_token_file must be configured")
		case len(c.HTTPClientConfig.BearerToken) > 0 || len(c.HTTPClientConfig.BearerTokenFile) > 0:
			return errors.New("marathon_sd: at most one of bearer_token, bearer_token_file, auth_token & auth_token_file must be configured")
		case c.HTTPClientConfig.Authorization != nil:
			return errors.New("marathon_sd: at most one of auth_token, auth_token_file & authorization must be configured")
		}
	}
	return c.HTTPClientConfig.Validate()
}

const appListPath string = "/v2/apps/?embed=apps.tasks"

// Discovery provides service discovery based on a Marathon instance.
type Discovery struct {
	*refresh.Discovery
	client      *http.Client
	servers     []string
	lastRefresh map[string]*targetgroup.Group
	appsClient  appListClient
}

// NewDiscovery returns a new Marathon Discovery.
func NewDiscovery(conf SDConfig, logger *slog.Logger, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*marathonMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "marathon_sd")
	if err != nil {
		return nil, err
	}

	switch {
	case len(conf.AuthToken) > 0:
		rt, err = newAuthTokenRoundTripper(conf.AuthToken, rt)
	case len(conf.AuthTokenFile) > 0:
		rt, err = newAuthTokenFileRoundTripper(conf.AuthTokenFile, rt)
	}
	if err != nil {
		return nil, err
	}

	d := &Discovery{
		client:     &http.Client{Transport: rt},
		servers:    conf.Servers,
		appsClient: fetchApps,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "marathon",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

type authTokenRoundTripper struct {
	authToken config.Secret
	rt        http.RoundTripper
}

// newAuthTokenRoundTripper adds the provided auth token to a request.
func newAuthTokenRoundTripper(token config.Secret, rt http.RoundTripper) (http.RoundTripper, error) {
	return &authTokenRoundTripper{token, rt}, nil
}

func (rt *authTokenRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	// According to https://docs.mesosphere.com/1.11/security/oss/managing-authentication/
	// DC/OS wants with "token=" a different Authorization header than implemented in httputil/client.go
	// so we set this explicitly here.
	request.Header.Set("Authorization", "token="+string(rt.authToken))

	return rt.rt.RoundTrip(request)
}

type authTokenFileRoundTripper struct {
	authTokenFile string
	rt            http.RoundTripper
}

// newAuthTokenFileRoundTripper adds the auth token read from the file to a request.
func newAuthTokenFileRoundTripper(tokenFile string, rt http.RoundTripper) (http.RoundTripper, error) {
	// fail-fast if we can't read the file.
	_, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read auth token file %s: %w", tokenFile, err)
	}
	return &authTokenFileRoundTripper{tokenFile, rt}, nil
}

func (rt *authTokenFileRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	b, err := os.ReadFile(rt.authTokenFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read auth token file %s: %w", rt.authTokenFile, err)
	}
	authToken := strings.TrimSpace(string(b))

	// According to https://docs.mesosphere.com/1.11/security/oss/managing-authentication/
	// DC/OS wants with "token=" a different Authorization header than implemented in httputil/client.go
	// so we set this explicitly here.
	request.Header.Set("Authorization", "token="+authToken)
	return rt.rt.RoundTrip(request)
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	targetMap, err := d.fetchTargetGroups(ctx)
	if err != nil {
		return nil, err
	}

	all := make([]*targetgroup.Group, 0, len(targetMap))
	for _, tg := range targetMap {
		all = append(all, tg)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Remove services which did disappear.
	for source := range d.lastRefresh {
		_, ok := targetMap[source]
		if !ok {
			all = append(all, &targetgroup.Group{Source: source})
		}
	}

	d.lastRefresh = targetMap
	return all, nil
}

func (d *Discovery) fetchTargetGroups(ctx context.Context) (map[string]*targetgroup.Group, error) {
	url := randomAppsURL(d.servers)
	apps, err := d.appsClient(ctx, d.client, url)
	if err != nil {
		return nil, err
	}

	groups := appsToTargetGroups(apps)
	return groups, nil
}

// task describes one instance of a service running on Marathon.
type task struct {
	ID          string      `json:"id"`
	Host        string      `json:"host"`
	Ports       []uint32    `json:"ports"`
	IPAddresses []ipAddress `json:"ipAddresses"`
}

// ipAddress describes the address and protocol the container's network interface is bound to.
type ipAddress struct {
	Address string `json:"ipAddress"`
	Proto   string `json:"protocol"`
}

// PortMapping describes in which port the process are binding inside the docker container.
type portMapping struct {
	Labels        map[string]string `json:"labels"`
	ContainerPort uint32            `json:"containerPort"`
	HostPort      uint32            `json:"hostPort"`
	ServicePort   uint32            `json:"servicePort"`
}

// DockerContainer describes a container which uses the docker runtime.
type dockerContainer struct {
	Image        string        `json:"image"`
	PortMappings []portMapping `json:"portMappings"`
}

// Container describes the runtime an app in running in.
type container struct {
	Docker       dockerContainer `json:"docker"`
	PortMappings []portMapping   `json:"portMappings"`
}

// PortDefinition describes which load balancer port you should access to access the service.
type portDefinition struct {
	Labels map[string]string `json:"labels"`
	Port   uint32            `json:"port"`
}

// Network describes the name and type of network the container is attached to.
type network struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

// App describes a service running on Marathon.
type app struct {
	ID              string            `json:"id"`
	Tasks           []task            `json:"tasks"`
	RunningTasks    int               `json:"tasksRunning"`
	Labels          map[string]string `json:"labels"`
	Container       container         `json:"container"`
	PortDefinitions []portDefinition  `json:"portDefinitions"`
	Networks        []network         `json:"networks"`
	RequirePorts    bool              `json:"requirePorts"`
}

// isContainerNet checks if the app's first network is set to mode 'container'.
func (app app) isContainerNet() bool {
	return len(app.Networks) > 0 && app.Networks[0].Mode == "container"
}

// appList is a list of Marathon apps.
type appList struct {
	Apps []app `json:"apps"`
}

// appListClient defines a function that can be used to get an application list from marathon.
type appListClient func(ctx context.Context, client *http.Client, url string) (*appList, error)

// fetchApps requests a list of applications from a marathon server.
func fetchApps(ctx context.Context, client *http.Client, url string) (*appList, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if (resp.StatusCode < 200) || (resp.StatusCode >= 300) {
		return nil, fmt.Errorf("non 2xx status '%v' response during marathon service discovery", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apps appList
	err = json.Unmarshal(b, &apps)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", url, err)
	}
	return &apps, nil
}

// randomAppsURL randomly selects a server from an array and creates
// an URL pointing to the app list.
func randomAppsURL(servers []string) string {
	// TODO: If possible update server list from Marathon at some point.
	server := servers[rand.Intn(len(servers))]
	return fmt.Sprintf("%s%s", server, appListPath)
}

// appsToTargetGroups takes an array of Marathon apps and converts them into target groups.
func appsToTargetGroups(apps *appList) map[string]*targetgroup.Group {
	tgroups := map[string]*targetgroup.Group{}
	for _, a := range apps.Apps {
		group := createTargetGroup(&a)
		tgroups[group.Source] = group
	}
	return tgroups
}

func createTargetGroup(app *app) *targetgroup.Group {
	var (
		targets = targetsForApp(app)
		appName = model.LabelValue(app.ID)
		image   = model.LabelValue(app.Container.Docker.Image)
	)
	tg := &targetgroup.Group{
		Targets: targets,
		Labels: model.LabelSet{
			appLabel:   appName,
			imageLabel: image,
		},
		Source: app.ID,
	}

	for ln, lv := range app.Labels {
		ln = appLabelPrefix + strutil.SanitizeLabelName(ln)
		tg.Labels[model.LabelName(ln)] = model.LabelValue(lv)
	}

	return tg
}

func targetsForApp(app *app) []model.LabelSet {
	targets := make([]model.LabelSet, 0, len(app.Tasks))

	var ports []uint32
	var labels []map[string]string
	var prefix string

	switch {
	case len(app.Container.PortMappings) != 0:
		// In Marathon 1.5.x the "container.docker.portMappings" object was moved
		// to "container.portMappings".
		ports, labels = extractPortMapping(app.Container.PortMappings, app.isContainerNet())
		prefix = portMappingLabelPrefix

	case len(app.Container.Docker.PortMappings) != 0:
		// Prior to Marathon 1.5 the port mappings could be found at the path
		// "container.docker.portMappings".
		ports, labels = extractPortMapping(app.Container.Docker.PortMappings, app.isContainerNet())
		prefix = portMappingLabelPrefix

	case len(app.PortDefinitions) != 0:
		// PortDefinitions deprecates the "ports" array and can be used to specify
		// a list of ports with metadata in case a mapping is not required.
		ports = make([]uint32, len(app.PortDefinitions))
		labels = make([]map[string]string, len(app.PortDefinitions))

		for i := 0; i < len(app.PortDefinitions); i++ {
			labels[i] = app.PortDefinitions[i].Labels
			// When requirePorts is false, this port becomes the 'servicePort', not the listen port.
			// In this case, the port needs to be taken from the task instead of the app.
			if app.RequirePorts {
				ports[i] = app.PortDefinitions[i].Port
			}
		}

		prefix = portDefinitionLabelPrefix
	}

	// Gather info about the app's 'tasks'. Each instance (container) is considered a task
	// and can be reachable at one or more host:port endpoints.
	for _, t := range app.Tasks {
		// There are no labels to gather if only Ports is defined. (eg. with host networking)
		// Ports can only be gathered from the Task (not from the app) and are guaranteed
		// to be the same across all tasks. If we haven't gathered any ports by now,
		// use the task's ports as the port list.
		if len(ports) == 0 && len(t.Ports) != 0 {
			ports = t.Ports
		}

		// Iterate over the ports we gathered using one of the methods above.
		for i, port := range ports {
			// A zero port here means that either the portMapping has a zero port defined,
			// or there is a portDefinition with requirePorts set to false. This means the port
			// is auto-generated by Mesos and needs to be looked up in the task.
			if port == 0 && len(t.Ports) == len(ports) {
				port = t.Ports[i]
			}

			// Each port represents a possible Prometheus target.
			targetAddress := targetEndpoint(&t, port, app.isContainerNet())
			target := model.LabelSet{
				model.AddressLabel: model.LabelValue(targetAddress),
				taskLabel:          model.LabelValue(t.ID),
				portIndexLabel:     model.LabelValue(strconv.Itoa(i)),
			}

			// Gather all port labels and set them on the current target, skip if the port has no Marathon labels.
			// This will happen in the host networking case with only `ports` defined, where
			// it is inefficient to allocate a list of possibly hundreds of empty label maps per host port.
			if len(labels) > 0 {
				for ln, lv := range labels[i] {
					ln = prefix + strutil.SanitizeLabelName(ln)
					target[model.LabelName(ln)] = model.LabelValue(lv)
				}
			}

			targets = append(targets, target)
		}
	}
	return targets
}

// Generate a target endpoint string in host:port format.
func targetEndpoint(task *task, port uint32, containerNet bool) string {
	var host string

	// Use the task's ipAddress field when it's in a container network
	if containerNet && len(task.IPAddresses) > 0 {
		host = task.IPAddresses[0].Address
	} else {
		host = task.Host
	}

	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}

// Get a list of ports and a list of labels from a PortMapping.
func extractPortMapping(portMappings []portMapping, containerNet bool) ([]uint32, []map[string]string) {
	ports := make([]uint32, len(portMappings))
	labels := make([]map[string]string, len(portMappings))

	for i := 0; i < len(portMappings); i++ {
		labels[i] = portMappings[i].Labels

		if containerNet {
			// If the app is in a container network, connect directly to the container port.
			ports[i] = portMappings[i].ContainerPort
		} else {
			// Otherwise, connect to the allocated host port for the container.
			// Note that this host port is likely set to 0 in the app definition, which means it is
			// automatically generated and needs to be extracted from the task's 'ports' array at a later stage.
			ports[i] = portMappings[i].HostPort
		}
	}

	return ports, labels
}
