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
	"net/http"
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
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/version"
	"golang.org/x/time/rate"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
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
	ociLabelVnicID             = ociLabel + "vnic_id"
	ociLabelPrivateIP          = ociLabel + "private_ip"
	ociLabelPublicIP           = ociLabel + "public_ip"
	ociLabelFreeformTagPrefix  = ociLabel + "tag_"
	ociLabelDefinedTagPrefix   = ociLabel + "defined_tag_"
)

const (
	authAPIKey            = "api_key"
	authInstancePrincipal = "instance_principal"

	// rateLimitRPS caps OCI API calls per second across all operations
	// (ListCompartments, ListInstances, ListVnicAttachments, GetVnic). OCI
	// throttles IAM read/list operations at 20/s on a free domain, and
	// ListCompartments is an IAM call, so the cap is kept below that ceiling
	// to avoid 429s. The OCI Go SDK's DefaultRetryPolicy already backs off on
	// 429 (TooManyRequests) and 5xx responses, so this is mainly a safety net
	// against runaway refreshes rather than a quota guard.
	// See https://docs.oracle.com/en-us/iaas/Content/Identity/sku/api-rate-limiting.htm.
	rateLimitRPS = 10.0

	// rateLimitBurst is the token-bucket burst size: the limiter releases up
	// to this many requests back-to-back before throttling to rateLimitRPS.
	rateLimitBurst = 50
)

// DefaultSDConfig is the default OCI service discovery configuration.
var DefaultSDConfig = SDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	Auth:             authAPIKey,
	HTTPClientConfig: config.DefaultHTTPClientConfig,
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
	Tenancy     string `yaml:"tenancy,omitempty"`
	User        string `yaml:"user,omitempty"`
	Fingerprint string `yaml:"fingerprint,omitempty"`
	KeyFile     string `yaml:"key_file,omitempty"`
	// KeyPassphrase is the passphrase used to decrypt KeyFile, if any.
	// Mutually exclusive with KeyPassphraseFile.
	KeyPassphrase config.Secret `yaml:"key_passphrase,omitempty"`
	// KeyPassphraseFile is a path to a file containing the passphrase used to
	// decrypt KeyFile. Mutually exclusive with KeyPassphrase.
	//
	// TODO: read this lazily via a custom common.ConfigurationProvider so
	// rotating the passphrase on disk takes effect without restarting
	// Prometheus, matching the runtime-reloadable semantics of password_file.
	KeyPassphraseFile string `yaml:"key_passphrase_file,omitempty"`

	// Region is required for all auth methods.
	Region string `yaml:"region"`

	// Compartments is an explicit list of compartment OCIDs to scan. When empty,
	// all active compartments reachable from the tenancy root are discovered
	// automatically via the OCI Identity API.
	Compartments []string `yaml:"compartments,omitempty"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

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
	c.HTTPClientConfig.SetDirectory(dir)
	c.KeyFile = config.JoinDir(dir, c.KeyFile)
	c.KeyPassphraseFile = config.JoinDir(dir, c.KeyPassphraseFile)
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
		return errors.New("oci_sd: region is required")
	}

	switch c.Auth {
	case authAPIKey:
		if c.Tenancy == "" {
			return errors.New("oci_sd: api_key auth requires tenancy")
		}
		if c.User == "" {
			return errors.New("oci_sd: api_key auth requires user")
		}
		if c.Fingerprint == "" {
			return errors.New("oci_sd: api_key auth requires fingerprint")
		}
		if c.KeyFile == "" {
			return errors.New("oci_sd: api_key auth requires key_file")
		}
		if c.KeyPassphrase != "" && c.KeyPassphraseFile != "" {
			return errors.New("oci_sd: at most one of key_passphrase and key_passphrase_file must be configured")
		}
	case authInstancePrincipal:
		if c.Tenancy != "" || c.User != "" || c.Fingerprint != "" || c.KeyFile != "" || c.KeyPassphrase != "" || c.KeyPassphraseFile != "" {
			return errors.New("oci_sd: instance_principal auth must not have tenancy, user, fingerprint, key_file, key_passphrase, or key_passphrase_file set")
		}
	default:
		return fmt.Errorf("oci_sd: unknown auth method %q, expected %q or %q", c.Auth, authAPIKey, authInstancePrincipal)
	}

	for i, cid := range c.Compartments {
		if cid == "" {
			return fmt.Errorf("oci_sd: compartments[%d] must not be empty", i)
		}
	}

	if c.RefreshInterval <= 0 {
		return fmt.Errorf("oci_sd: refresh_interval must be positive, got %s", c.RefreshInterval)
	}

	return c.HTTPClientConfig.Validate()
}

// compartmentLister returns the ordered list of compartment OCIDs to scan.
// When compartments are configured explicitly it returns them directly; when
// auto-discovering it recursively lists all active compartments from the
// tenancy root via the OCI Identity API.
type compartmentLister func(ctx context.Context) ([]string, error)

// instancesLister lists compute instances in a compartment, handling pagination
// internally. The concrete ComputeClient is captured in the closure so it is
// not reachable through reflection on the interface-boxed Discovery struct,
// keeping dead-code elimination effective and the binary small.
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
	port                int
	logger              *slog.Logger
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, opts discovery.DiscovererOptions) (*Discovery, error) {
	m, ok := opts.Metrics.(*ociMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}

	httpClient, err := config.NewClientFromConfig(conf.HTTPClientConfig, "oci_sd")
	if err != nil {
		return nil, fmt.Errorf("oci_sd: failed to build HTTP client: %w", err)
	}

	provider, err := newConfigurationProvider(conf, httpClient)
	if err != nil {
		return nil, fmt.Errorf("oci_sd: failed to build configuration provider: %w", err)
	}

	tenancy, err := provider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("oci_sd: failed to determine tenancy OCID: %w", err)
	}

	// Rate limiter and retry policy are shared across all closures so every OCI
	// API call counts against the same budget. The burst is kept small so a
	// cold refresh does not fire a full second's worth of calls at once.
	limiter := rate.NewLimiter(rate.Limit(rateLimitRPS), rateLimitBurst)
	retryPolicy := common.DefaultRetryPolicy()

	compLister, err := newCompartmentLister(provider, httpClient, tenancy, conf.Compartments, limiter, &retryPolicy, opts.Logger)
	if err != nil {
		return nil, fmt.Errorf("oci_sd: failed to create compartment lister: %w", err)
	}

	lister, attachmentLister, fetcher, err := newOCIClients(provider, httpClient, limiter, &retryPolicy)
	if err != nil {
		return nil, fmt.Errorf("oci_sd: failed to create clients: %w", err)
	}

	d := &Discovery{
		listCompartments:    compLister,
		listInstances:       lister,
		listVnicAttachments: attachmentLister,
		getVnic:             fetcher,
		tenancy:             tenancy,
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
// httpClient is used by the instance-principal key provider so that the IMDS
// metadata call honours the user-supplied Prometheus HTTP knobs (proxy, TLS,
// custom CA). For API key auth the provider is purely local so the HTTP client
// is unused here.
func newConfigurationProvider(conf *SDConfig, httpClient *http.Client) (common.ConfigurationProvider, error) {
	switch conf.Auth {
	case authInstancePrincipal:
		return auth.InstancePrincipalConfigurationProviderWithCustomClient(
			func(common.HTTPRequestDispatcher) (common.HTTPRequestDispatcher, error) {
				return httpClient, nil
			},
		)
	default: // authAPIKey
		keyPEM, err := os.ReadFile(conf.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("reading OCI key file %q: %w", conf.KeyFile, err)
		}
		passphrase, err := resolveKeyPassphrase(conf)
		if err != nil {
			return nil, err
		}
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

// resolveKeyPassphrase reads the inline KeyPassphrase or the contents of
// KeyPassphraseFile. The file is read eagerly at construction time; rotation
// at runtime is not yet supported (see TODO on SDConfig.KeyPassphraseFile).
func resolveKeyPassphrase(conf *SDConfig) (string, error) {
	if conf.KeyPassphraseFile != "" {
		b, err := os.ReadFile(conf.KeyPassphraseFile)
		if err != nil {
			return "", fmt.Errorf("reading OCI key passphrase file %q: %w", conf.KeyPassphraseFile, err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	return string(conf.KeyPassphrase), nil
}

// compartmentPaginator lists the direct child compartments of parentID one
// page at a time. It is the smallest surface listAllCompartments needs from
// the OCI Identity API, which keeps the walk easy to test with a fake.
type compartmentPaginator func(ctx context.Context, parentID string, page *string) ([]identity.Compartment, *string, error)

// newCompartmentLister returns a compartmentLister that either returns the
// explicit list from the config directly, or auto-discovers all active
// compartments under the tenancy root via the OCI Identity API.
//
// When compartments are explicitly configured no Identity client is created,
// so the identity package methods are dropped by dead-code elimination.
func newCompartmentLister(provider common.ConfigurationProvider, httpClient *http.Client, tenancy string, compartments []string, limiter *rate.Limiter, retryPolicy *common.RetryPolicy, logger *slog.Logger) (compartmentLister, error) {
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
	idClient.HTTPClient = httpClient
	idClient.UserAgent = version.PrometheusUserAgent()

	paginate := func(ctx context.Context, parentID string, page *string) ([]identity.Compartment, *string, error) {
		if err := limiter.Wait(ctx); err != nil {
			return nil, nil, err
		}
		resp, err := idClient.ListCompartments(ctx, identity.ListCompartmentsRequest{
			CompartmentId:          common.String(parentID),
			AccessLevel:            identity.ListCompartmentsAccessLevelAccessible,
			CompartmentIdInSubtree: common.Bool(false), // Direct children only.
			Limit:                  common.Int(100),
			Page:                   page,
			RequestMetadata:        common.RequestMetadata{RetryPolicy: retryPolicy},
		})
		if err != nil {
			return nil, nil, err
		}
		return resp.Items, resp.OpcNextPage, nil
	}

	return func(ctx context.Context) ([]string, error) {
		return listAllCompartments(ctx, paginate, tenancy, logger)
	}, nil
}

// listAllCompartments recursively discovers all active compartments reachable
// from root using a BFS queue, including root itself. Child compartments that
// fail to list (typically a permission gap) are logged and skipped so a single
// gap does not abort the walk. Context cancellation aborts the walk immediately
// rather than being treated as a permission gap.
func listAllCompartments(ctx context.Context, paginate compartmentPaginator, root string, logger *slog.Logger) ([]string, error) {
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
			items, next, err := paginate(ctx, current, page)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				logger.Warn("OCI SD failed to list child compartments, skipping subtree",
					"compartment_id", current, "err", err)
				break
			}
			for _, comp := range items {
				if comp.Id != nil && comp.LifecycleState == identity.CompartmentLifecycleStateActive && !visited[*comp.Id] {
					queue = append(queue, *comp.Id)
				}
			}
			if next == nil {
				break
			}
			page = next
		}
	}
	return all, nil
}

// newOCIClients constructs the three closure functions that wrap the OCI SDK
// clients. The concrete client values live only in the closure context so
// reflection cannot reach them through the interface-boxed Discovery struct,
// allowing the linker to drop unused SDK operations and keep the binary small.
func newOCIClients(provider common.ConfigurationProvider, httpClient *http.Client, limiter *rate.Limiter, retryPolicy *common.RetryPolicy) (instancesLister, vnicAttachmentLister, vnicFetcher, error) {
	computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating OCI compute client: %w", err)
	}
	computeClient.HTTPClient = httpClient
	computeClient.UserAgent = version.PrometheusUserAgent()

	vnetClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating OCI virtual network client: %w", err)
	}
	vnetClient.HTTPClient = httpClient
	vnetClient.UserAgent = version.PrometheusUserAgent()

	lister := func(ctx context.Context, compartmentID string) ([]core.Instance, error) {
		var instances []core.Instance
		req := core.ListInstancesRequest{
			CompartmentId:   common.String(compartmentID),
			Limit:           common.Int(100),
			RequestMetadata: common.RequestMetadata{RetryPolicy: retryPolicy},
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
		return nil, fmt.Errorf("oci_sd: failed to list compartments: %w", err)
	}

	// Merge all compartments into a single target group so the manager
	// replaces the whole target set every refresh.
	tg := &targetgroup.Group{Source: "OCI"}

	for _, compartmentID := range compartments {
		instances, err := d.listInstances(ctx, compartmentID)
		if err != nil {
			// Return the error rather than emitting a partial group, so the
			// refresh framework keeps the last known good targets on a
			// transient OCI API failure.
			return nil, fmt.Errorf("listing OCI instances in compartment %s: %w", compartmentID, err)
		}

		for i := range instances {
			inst := &instances[i]

			if inst.Id == nil || inst.CompartmentId == nil {
				d.logger.Warn("OCI SD skipping instance with missing OCID fields",
					"compartment_id", compartmentID)
				continue
			}

			vnics, err := d.resolveVnics(ctx, inst)
			if err != nil {
				d.logger.Warn("OCI SD failed to resolve VNIC IPs, skipping instance",
					"instance_id", *inst.Id, "err", err)
				continue
			}

			if vnics.primary.privateIP == "" && vnics.primary.publicIP == "" {
				d.logger.Debug("OCI SD skipping instance with no usable IP address",
					"instance_id", *inst.Id)
				continue
			}

			tg.Targets = append(tg.Targets, instanceLabels(inst, vnics, d.tenancy, d.port))
		}
	}

	return []*targetgroup.Group{tg}, nil
}

// vnicInfo is the subset of a single OCI VNIC that the SD exposes through
// meta labels.
type vnicInfo struct {
	id        string
	privateIP string
	publicIP  string
}

// instanceVnics holds the primary VNIC resolved for one compute instance. The
// primary VNIC supplies the scrape address and the VNIC meta labels.
type instanceVnics struct {
	primary vnicInfo
}

// resolveVnics returns the primary VNIC for inst. Each attachment costs one
// GetVnic call and OCI offers no batch VNIC fetch, so the walk stops as soon
// as the primary VNIC is found rather than fanning out a call per attachment.
// Caller must guarantee inst.Id and inst.CompartmentId are non-nil.
func (d *Discovery) resolveVnics(ctx context.Context, inst *core.Instance) (instanceVnics, error) {
	attachments, err := d.listVnicAttachments(ctx, stringVal(inst.CompartmentId), stringVal(inst.Id))
	if err != nil {
		return instanceVnics{}, fmt.Errorf("listing VNIC attachments for instance %s: %w", stringVal(inst.Id), err)
	}

	var out instanceVnics
	for _, att := range attachments {
		if att.VnicId == nil || att.LifecycleState != core.VnicAttachmentLifecycleStateAttached {
			continue
		}
		vnic, err := d.getVnic(ctx, *att.VnicId)
		if err != nil {
			return instanceVnics{}, fmt.Errorf("fetching VNIC %s: %w", *att.VnicId, err)
		}
		if vnic == nil {
			continue
		}
		if vnic.IsPrimary != nil && *vnic.IsPrimary {
			out.primary = vnicInfo{
				id:        stringVal(vnic.Id),
				privateIP: stringVal(vnic.PrivateIp),
				publicIP:  stringVal(vnic.PublicIp),
			}
			break
		}
	}

	return out, nil
}

// instanceLabels builds the label set for one OCI compute instance.
func instanceLabels(inst *core.Instance, vnics instanceVnics, tenancy string, port int) model.LabelSet {
	addr := vnics.primary.privateIP
	if addr == "" {
		addr = vnics.primary.publicIP
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
		ociLabelVnicID:             model.LabelValue(vnics.primary.id),
		ociLabelPrivateIP:          model.LabelValue(vnics.primary.privateIP),
		ociLabelPublicIP:           model.LabelValue(vnics.primary.publicIP),
	}

	for k, v := range inst.FreeformTags {
		labels[model.LabelName(ociLabelFreeformTagPrefix+strutil.SanitizeLabelName(k))] = model.LabelValue(v)
	}

	// OCI's typed tag system allows string, integer, and boolean values for
	// defined tags. Stringify scalar values so a keep/drop relabel on a
	// numeric or boolean tag does not silently match nothing. Non-scalar
	// values (slices, maps) cannot be safely represented as a single label
	// value and are skipped.
	for ns, tags := range inst.DefinedTags {
		for k, v := range tags {
			s, ok := stringifyDefinedTag(v)
			if !ok {
				continue
			}
			name := ociLabelDefinedTagPrefix + strutil.SanitizeLabelName(ns) + "_" + strutil.SanitizeLabelName(k)
			labels[model.LabelName(name)] = model.LabelValue(s)
		}
	}

	return labels
}

// stringifyDefinedTag converts a scalar OCI defined-tag value to a string.
// The SDK decodes defined tags as map[string]any: JSON numbers arrive as
// float64, booleans as bool, and strings as string. Integer fields are kept
// (some callers may construct DefinedTags in Go directly). Non-scalar
// values cannot be safely flattened to a label and are reported as not OK.
func stringifyDefinedTag(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32), true
	case int:
		return strconv.FormatInt(int64(t), 10), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case int32:
		return strconv.FormatInt(int64(t), 10), true
	default:
		return "", false
	}
}

// stringVal dereferences a *string, returning "" if nil.
func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
