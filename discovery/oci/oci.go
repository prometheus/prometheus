// Copyright The Prometheus Authors
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

// Package oci provides service discovery for Oracle Cloud Infrastructure compute instances.
package oci

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"golang.org/x/time/rate"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	ociLabel                   = model.MetaLabelPrefix + "oci_"
	ociLabelInstanceID         = ociLabel + "instance_id"
	ociLabelInstanceName       = ociLabel + "instance_name"
	ociLabelInstanceState      = ociLabel + "instance_state"
	ociLabelInstanceShape      = ociLabel + "instance_shape"
	ociLabelAvailabilityDomain = ociLabel + "availability_domain"
	ociLabelFaultDomain        = ociLabel + "fault_domain"
	ociLabelRegion             = ociLabel + "region"
	ociLabelTenancyID          = ociLabel + "tenancy_id"
	ociLabelCompartmentID      = ociLabel + "compartment_id"
	ociLabelImageID            = ociLabel + "image_id"
	ociLabelPrivateIP          = ociLabel + "private_ip"
	ociLabelPublicIP           = ociLabel + "public_ip"
	ociLabelFreeformTagPrefix  = ociLabel + "tag_"
	ociLabelDefinedTagPrefix   = ociLabel + "defined_tag_"
)

const (
	authAPIKey            = "api_key"
	authInstancePrincipal = "instance_principal"

	tagFilterActionInclude = "include"
	tagFilterActionExclude = "exclude"
)

// DefaultSDConfig is the default OCI service discovery configuration.
var DefaultSDConfig = SDConfig{
	Port:            80,
	RefreshInterval: model.Duration(60 * time.Second),
	Auth:            authAPIKey,
	TagFilterAction: tagFilterActionInclude,
	RateLimitRPS:    10.0,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for OCI service discovery.
type SDConfig struct {
	// Auth selects the authentication method. Valid values: "api_key" (default),
	// "instance_principal".
	Auth string `yaml:"auth,omitempty"`

	// API key auth fields. Required when Auth is "api_key".
	Tenancy       string        `yaml:"tenancy,omitempty"`
	User          string        `yaml:"user,omitempty"`
	Fingerprint   string        `yaml:"fingerprint,omitempty"`
	KeyFile       string        `yaml:"key_file,omitempty"`
	KeyPassphrase config.Secret `yaml:"key_passphrase,omitempty"`

	// Region is required for all auth methods.
	Region string `yaml:"region"`

	// Compartments is an explicit list of compartment OCIDs to scan. When empty,
	// all active compartments reachable from the tenancy root are discovered
	// automatically via the OCI Identity API.
	Compartments []string `yaml:"compartments,omitempty"`

	// Filter is an optional OCI display-name substring filter for ListInstances.
	Filter string `yaml:"filter,omitempty"`

	// TagFilterKey and TagFilterValue together select instances by a freeform or
	// defined tag. TagFilterAction controls whether matching instances are included
	// (default: "include") or excluded ("exclude"). When TagFilterKey is empty all
	// instances are returned regardless of TagFilterValue and TagFilterAction.
	TagFilterKey    string `yaml:"tag_filter_key,omitempty"`
	TagFilterValue  string `yaml:"tag_filter_value,omitempty"`
	TagFilterAction string `yaml:"tag_filter_action,omitempty"`

	// RateLimitRPS caps the number of OCI API calls per second to avoid hitting
	// per-tenancy service limits. Defaults to 10.
	RateLimitRPS float64 `yaml:"rate_limit_rps,omitempty"`

	Port            int            `yaml:"port"`
	RefreshInterval model.Duration `yaml:"refresh_interval"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &ociMetrics{refreshMetrics: rmi}
}

// Name returns the name of the OCI service discovery config.
func (*SDConfig) Name() string { return "oci" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	if c.KeyFile != "" && !strings.HasPrefix(c.KeyFile, "/") {
		c.KeyFile = dir + "/" + c.KeyFile
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.validate()
}

// validate checks all SDConfig fields for consistency. It is called from
// UnmarshalYAML and is also used directly in tests.
func (c *SDConfig) validate() error {
	if c.Region == "" {
		return errors.New("OCI SD configuration requires a region")
	}

	switch c.Auth {
	case authAPIKey:
		if c.Tenancy == "" {
			return errors.New("OCI SD api_key auth requires tenancy")
		}
		if c.User == "" {
			return errors.New("OCI SD api_key auth requires user")
		}
		if c.Fingerprint == "" {
			return errors.New("OCI SD api_key auth requires fingerprint")
		}
		if c.KeyFile == "" {
			return errors.New("OCI SD api_key auth requires key_file")
		}
	case authInstancePrincipal:
		if c.Tenancy != "" || c.User != "" || c.Fingerprint != "" || c.KeyFile != "" {
			return errors.New("OCI SD instance_principal auth must not have tenancy, user, fingerprint, or key_file set")
		}
	default:
		return fmt.Errorf("OCI SD unknown auth method %q, expected %q or %q", c.Auth, authAPIKey, authInstancePrincipal)
	}

	for i, cid := range c.Compartments {
		if cid == "" {
			return fmt.Errorf("OCI SD compartments[%d] must not be empty", i)
		}
	}

	if c.TagFilterKey != "" {
		if c.TagFilterValue == "" {
			return errors.New("OCI SD tag_filter_key requires tag_filter_value")
		}
		switch c.TagFilterAction {
		case tagFilterActionInclude, tagFilterActionExclude:
		default:
			return fmt.Errorf("OCI SD tag_filter_action must be %q or %q, got %q", tagFilterActionInclude, tagFilterActionExclude, c.TagFilterAction)
		}
	}

	if c.RateLimitRPS <= 0 {
		return errors.New("OCI SD rate_limit_rps must be positive")
	}

	return nil
}

// compartmentLister returns the ordered list of compartment OCIDs to scan.
// When compartments are configured explicitly it returns them directly; when
// auto-discovering it recursively lists all active compartments from the
// tenancy root via the OCI Identity API.
type compartmentLister func(ctx context.Context) ([]string, error)

// instancesLister lists RUNNING compute instances in a compartment, handling
// pagination internally. The concrete ComputeClient is captured in the closure
// so it is not reachable through reflection on the interface-boxed Discovery
// struct, keeping dead-code elimination effective and the binary small.
type instancesLister func(ctx context.Context, compartmentID string) ([]core.Instance, error)

// vnicAttachmentLister lists VNIC attachments for an instance in a compartment.
type vnicAttachmentLister func(ctx context.Context, compartmentID, instanceID string) ([]core.VnicAttachment, error)

// vnicFetcher fetches the details of a single VNIC by its ID.
type vnicFetcher func(ctx context.Context, vnicID string) (*core.Vnic, error)

// Discovery periodically performs OCI compute instance discovery. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	listCompartments    compartmentLister
	listInstances       instancesLister
	listVnicAttachments vnicAttachmentLister
	getVnic             vnicFetcher
	tenancy             string
	tagFilterKey        string
	tagFilterValue      string
	tagFilterAction     string
	port                int
	logger              *slog.Logger
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, opts discovery.DiscovererOptions) (*Discovery, error) {
	m, ok := opts.Metrics.(*ociMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	provider, err := newConfigurationProvider(conf)
	if err != nil {
		return nil, fmt.Errorf("OCI SD failed to build configuration provider: %w", err)
	}

	tenancy, err := provider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("OCI SD failed to determine tenancy OCID: %w", err)
	}

	// Rate limiter and retry policy are shared across all closures so every OCI
	// API call counts against the same budget.
	limiter := rate.NewLimiter(rate.Limit(conf.RateLimitRPS), max(1, int(conf.RateLimitRPS)))
	retryPolicy := common.DefaultRetryPolicy()

	compLister, err := newCompartmentLister(provider, tenancy, conf.Compartments, limiter, &retryPolicy)
	if err != nil {
		return nil, fmt.Errorf("OCI SD failed to create compartment lister: %w", err)
	}

	lister, attachmentLister, fetcher, err := newOCIClients(provider, conf.Filter, limiter, &retryPolicy)
	if err != nil {
		return nil, fmt.Errorf("OCI SD failed to create clients: %w", err)
	}

	d := &Discovery{
		listCompartments:    compLister,
		listInstances:       lister,
		listVnicAttachments: attachmentLister,
		getVnic:             fetcher,
		tenancy:             tenancy,
		tagFilterKey:        conf.TagFilterKey,
		tagFilterValue:      conf.TagFilterValue,
		tagFilterAction:     conf.TagFilterAction,
		port:                conf.Port,
		logger:              opts.Logger,
	}

	d.Discovery = refresh.NewDiscovery(refresh.Options{
		Logger:              opts.Logger,
		Mech:                "oci",
		SetName:             opts.SetName,
		Interval:            time.Duration(conf.RefreshInterval),
		RefreshF:            d.refresh,
		MetricsInstantiator: m.refreshMetrics,
	})

	return d, nil
}

// newConfigurationProvider builds an OCI ConfigurationProvider for the given SDConfig.
func newConfigurationProvider(conf *SDConfig) (common.ConfigurationProvider, error) {
	switch conf.Auth {
	case authInstancePrincipal:
		return auth.InstancePrincipalConfigurationProvider()
	default: // authAPIKey
		keyPEM, err := os.ReadFile(conf.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("reading OCI key file %q: %w", conf.KeyFile, err)
		}
		passphrase := string(conf.KeyPassphrase)
		var passphrasePtr *string
		if passphrase != "" {
			passphrasePtr = &passphrase
		}
		return common.NewRawConfigurationProvider(
			conf.Tenancy, conf.User, conf.Region,
			conf.Fingerprint, string(keyPEM), passphrasePtr,
		), nil
	}
}

// newCompartmentLister returns a compartmentLister that either returns the
// explicit list from the config directly, or auto-discovers all active
// compartments under the tenancy root via the OCI Identity API.
//
// When compartments are explicitly configured no Identity client is created,
// so the identity package methods are dropped by dead-code elimination.
func newCompartmentLister(provider common.ConfigurationProvider, tenancy string, compartments []string, limiter *rate.Limiter, retryPolicy *common.RetryPolicy) (compartmentLister, error) {
	if len(compartments) > 0 {
		ids := make([]string, len(compartments))
		copy(ids, compartments)
		return func(_ context.Context) ([]string, error) {
			return ids, nil
		}, nil
	}

	// Auto-discovery: the Identity client lives only in the closure context so
	// reflection cannot traverse it through the Discovery struct.
	idClient, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("creating OCI identity client: %w", err)
	}

	return func(ctx context.Context) ([]string, error) {
		return listAllCompartments(ctx, &idClient, tenancy, limiter, retryPolicy)
	}, nil
}

// listAllCompartments recursively discovers all active compartments reachable
// from root using a BFS queue, including root itself. Child compartments that
// fail to list are skipped so a single permission gap does not abort the walk.
func listAllCompartments(ctx context.Context, idClient *identity.IdentityClient, root string, limiter *rate.Limiter, retryPolicy *common.RetryPolicy) ([]string, error) {
	var all []string
	visited := make(map[string]bool)
	queue := []string{root}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true
		all = append(all, current)

		var page *string
		for {
			if err := limiter.Wait(ctx); err != nil {
				return nil, err
			}
			resp, err := idClient.ListCompartments(ctx, identity.ListCompartmentsRequest{
				CompartmentId:          common.String(current),
				AccessLevel:            identity.ListCompartmentsAccessLevelAccessible,
				CompartmentIdInSubtree: common.Bool(false), // direct children only
				Limit:                  common.Int(100),
				Page:                   page,
				RequestMetadata:        common.RequestMetadata{RetryPolicy: retryPolicy},
			})
			if err != nil {
				// Permission gap on this compartment - skip its children.
				break
			}
			for _, comp := range resp.Items {
				if comp.Id != nil && comp.LifecycleState == identity.CompartmentLifecycleStateActive && !visited[*comp.Id] {
					queue = append(queue, *comp.Id)
				}
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = resp.OpcNextPage
		}
	}
	return all, nil
}

// newOCIClients constructs the three closure functions that wrap the OCI SDK
// clients. The concrete client values live only in the closure context so
// reflection cannot reach them through the interface-boxed Discovery struct,
// allowing the linker to drop unused SDK operations and keep the binary small.
func newOCIClients(provider common.ConfigurationProvider, filter string, limiter *rate.Limiter, retryPolicy *common.RetryPolicy) (instancesLister, vnicAttachmentLister, vnicFetcher, error) {
	computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating OCI compute client: %w", err)
	}

	vnetClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating OCI virtual network client: %w", err)
	}

	lister := func(ctx context.Context, compartmentID string) ([]core.Instance, error) {
		var instances []core.Instance
		req := core.ListInstancesRequest{
			CompartmentId:   common.String(compartmentID),
			LifecycleState:  core.InstanceLifecycleStateRunning,
			Limit:           common.Int(100),
			RequestMetadata: common.RequestMetadata{RetryPolicy: retryPolicy},
		}
		if filter != "" {
			req.DisplayName = common.String(filter)
		}
		for {
			if err := limiter.Wait(ctx); err != nil {
				return nil, err
			}
			resp, err := computeClient.ListInstances(ctx, req)
			if err != nil {
				return nil, err
			}
			instances = append(instances, resp.Items...)
			if resp.OpcNextPage == nil {
				break
			}
			req.Page = resp.OpcNextPage
		}
		return instances, nil
	}

	attachmentLister := func(ctx context.Context, compartmentID, instanceID string) ([]core.VnicAttachment, error) {
		var attachments []core.VnicAttachment
		req := core.ListVnicAttachmentsRequest{
			CompartmentId:   common.String(compartmentID),
			InstanceId:      common.String(instanceID),
			Limit:           common.Int(100),
			RequestMetadata: common.RequestMetadata{RetryPolicy: retryPolicy},
		}
		for {
			if err := limiter.Wait(ctx); err != nil {
				return nil, err
			}
			resp, err := computeClient.ListVnicAttachments(ctx, req)
			if err != nil {
				return nil, err
			}
			attachments = append(attachments, resp.Items...)
			if resp.OpcNextPage == nil {
				break
			}
			req.Page = resp.OpcNextPage
		}
		return attachments, nil
	}

	fetcher := func(ctx context.Context, vnicID string) (*core.Vnic, error) {
		if err := limiter.Wait(ctx); err != nil {
			return nil, err
		}
		resp, err := vnetClient.GetVnic(ctx, core.GetVnicRequest{
			VnicId:          common.String(vnicID),
			RequestMetadata: common.RequestMetadata{RetryPolicy: retryPolicy},
		})
		if err != nil {
			return nil, err
		}
		return &resp.Vnic, nil
	}

	return lister, attachmentLister, fetcher, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	compartments, err := d.listCompartments(ctx)
	if err != nil {
		return nil, fmt.Errorf("OCI SD failed to list compartments: %w", err)
	}

	var tgs []*targetgroup.Group

	for _, compartmentID := range compartments {
		tg := &targetgroup.Group{Source: "OCI_" + compartmentID}

		instances, err := d.listInstances(ctx, compartmentID)
		if err != nil {
			d.logger.Warn("OCI SD failed to list instances, skipping compartment",
				"compartment_id", compartmentID, "err", err)
			continue
		}

		for i := range instances {
			inst := &instances[i]

			// Tag filter is applied before VNIC resolution to avoid unnecessary API
			// calls for instances that will be dropped anyway.
			if d.tagFilterKey != "" {
				match := hasTag(inst, d.tagFilterKey, d.tagFilterValue)
				if d.tagFilterAction == tagFilterActionInclude && !match {
					continue
				}
				if d.tagFilterAction == tagFilterActionExclude && match {
					continue
				}
			}

			privateIP, publicIP, err := d.primaryVnicIPs(ctx, inst)
			if err != nil {
				d.logger.Warn("OCI SD failed to resolve VNIC IPs, skipping instance",
					"instance_id", stringVal(inst.Id), "err", err)
				continue
			}

			tg.Targets = append(tg.Targets, instanceLabels(inst, privateIP, publicIP, d.tenancy, d.port))
		}

		tgs = append(tgs, tg)
	}

	return tgs, nil
}

// primaryVnicIPs returns the private and public IP of the primary VNIC for inst.
// publicIP may be empty if no public IP is assigned.
func (d *Discovery) primaryVnicIPs(ctx context.Context, inst *core.Instance) (privateIP, publicIP string, err error) {
	attachments, err := d.listVnicAttachments(ctx, *inst.CompartmentId, *inst.Id)
	if err != nil {
		return "", "", fmt.Errorf("listing VNIC attachments for instance %s: %w", *inst.Id, err)
	}

	for _, att := range attachments {
		if att.VnicId == nil || att.LifecycleState != core.VnicAttachmentLifecycleStateAttached {
			continue
		}
		vnic, err := d.getVnic(ctx, *att.VnicId)
		if err != nil {
			return "", "", fmt.Errorf("fetching VNIC %s: %w", *att.VnicId, err)
		}
		if vnic.IsPrimary == nil || !*vnic.IsPrimary {
			continue
		}
		if vnic.PrivateIp != nil {
			privateIP = *vnic.PrivateIp
		}
		if vnic.PublicIp != nil {
			publicIP = *vnic.PublicIp
		}
		return privateIP, publicIP, nil
	}

	return "", "", nil
}

// hasTag reports whether inst carries a freeform or defined tag with the given
// key and value. Freeform tags are checked first; defined tags are checked
// across all namespaces. Only string-typed defined tag values are compared.
func hasTag(inst *core.Instance, key, value string) bool {
	if v, ok := inst.FreeformTags[key]; ok && v == value {
		return true
	}
	for _, nsMap := range inst.DefinedTags {
		if v, ok := nsMap[key]; ok {
			if s, ok := v.(string); ok && s == value {
				return true
			}
		}
	}
	return false
}

// instanceLabels builds the label set for one OCI compute instance.
func instanceLabels(inst *core.Instance, privateIP, publicIP, tenancy string, port int) model.LabelSet {
	addr := privateIP
	if addr == "" {
		addr = publicIP
	}

	labels := model.LabelSet{
		model.AddressLabel:         model.LabelValue(net.JoinHostPort(addr, strconv.Itoa(port))),
		ociLabelInstanceID:         model.LabelValue(stringVal(inst.Id)),
		ociLabelInstanceName:       model.LabelValue(stringVal(inst.DisplayName)),
		ociLabelInstanceState:      model.LabelValue(string(inst.LifecycleState)),
		ociLabelInstanceShape:      model.LabelValue(stringVal(inst.Shape)),
		ociLabelAvailabilityDomain: model.LabelValue(stringVal(inst.AvailabilityDomain)),
		ociLabelFaultDomain:        model.LabelValue(stringVal(inst.FaultDomain)),
		ociLabelRegion:             model.LabelValue(stringVal(inst.Region)),
		ociLabelTenancyID:          model.LabelValue(tenancy),
		ociLabelCompartmentID:      model.LabelValue(stringVal(inst.CompartmentId)),
		ociLabelImageID:            model.LabelValue(stringVal(inst.ImageId)),
		ociLabelPrivateIP:          model.LabelValue(privateIP),
		ociLabelPublicIP:           model.LabelValue(publicIP),
	}

	for k, v := range inst.FreeformTags {
		labels[model.LabelName(ociLabelFreeformTagPrefix+sanitizeLabelName(k))] = model.LabelValue(v)
	}

	// Only string-typed defined tag values are emitted; numeric and boolean
	// values from OCI's typed tag system are skipped to avoid label churn.
	for ns, tags := range inst.DefinedTags {
		for k, v := range tags {
			s, ok := v.(string)
			if !ok {
				continue
			}
			name := ociLabelDefinedTagPrefix + sanitizeLabelName(ns) + "_" + sanitizeLabelName(k)
			labels[model.LabelName(name)] = model.LabelValue(s)
		}
	}

	return labels
}

// sanitizeLabelName replaces characters that are invalid in Prometheus label
// names with underscores and lowercases the result.
func sanitizeLabelName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// stringVal dereferences a *string, returning "" if nil.
func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
