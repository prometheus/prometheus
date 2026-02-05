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

package aws

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	lightsailLabel                    = model.MetaLabelPrefix + "lightsail_"
	lightsailLabelAZ                  = lightsailLabel + "availability_zone"
	lightsailLabelBlueprintID         = lightsailLabel + "blueprint_id"
	lightsailLabelBundleID            = lightsailLabel + "bundle_id"
	lightsailLabelInstanceName        = lightsailLabel + "instance_name"
	lightsailLabelInstanceState       = lightsailLabel + "instance_state"
	lightsailLabelInstanceSupportCode = lightsailLabel + "instance_support_code"
	lightsailLabelIPv6Addresses       = lightsailLabel + "ipv6_addresses"
	lightsailLabelPrivateIP           = lightsailLabel + "private_ip"
	lightsailLabelPublicIP            = lightsailLabel + "public_ip"
	lightsailLabelRegion              = lightsailLabel + "region"
	lightsailLabelTag                 = lightsailLabel + "tag_"
	lightsailLabelSeparator           = ","
)

// DefaultLightsailSDConfig is the default Lightsail SD configuration.
var DefaultLightsailSDConfig = LightsailSDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&LightsailSDConfig{})
}

// LightsailSDConfig is the configuration for Lightsail based service discovery.
type LightsailSDConfig struct {
	Endpoint        string         `yaml:"endpoint"`
	Region          string         `yaml:"region"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       config.Secret  `yaml:"secret_key,omitempty"`
	Profile         string         `yaml:"profile,omitempty"`
	RoleARN         string         `yaml:"role_arn,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*LightsailSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &lightsailMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Lightsail Config.
func (*LightsailSDConfig) Name() string { return "lightsail" }

// NewDiscoverer returns a Discoverer for the Lightsail Config.
func (c *LightsailSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewLightsailDiscovery(c, opts)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the Lightsail Config.
func (c *LightsailSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultLightsailSDConfig
	type plain LightsailSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	c.Region, err = loadRegion(context.Background(), c.Region)
	if err != nil {
		return fmt.Errorf("could not determine AWS region: %w", err)
	}

	return c.HTTPClientConfig.Validate()
}

// LightsailDiscovery periodically performs Lightsail-SD requests. It implements
// the Discoverer interface.
type LightsailDiscovery struct {
	*refresh.Discovery
	cfg       *LightsailSDConfig
	lightsail *lightsail.Client
}

// NewLightsailDiscovery returns a new LightsailDiscovery which periodically refreshes its targets.
func NewLightsailDiscovery(conf *LightsailSDConfig, opts discovery.DiscovererOptions) (*LightsailDiscovery, error) {
	m, ok := opts.Metrics.(*lightsailMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}

	d := &LightsailDiscovery{
		cfg: conf,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "lightsail",
			SetName:             opts.SetName,
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *LightsailDiscovery) lightsailClient(ctx context.Context) (*lightsail.Client, error) {
	if d.lightsail != nil {
		return d.lightsail, nil
	}

	// Build the HTTP client from the provided HTTPClientConfig.
	httpClient, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "lightsail_sd")
	if err != nil {
		return nil, err
	}

	// Build the AWS config with the provided region.
	configOptions := []func(*awsConfig.LoadOptions) error{
		awsConfig.WithRegion(d.cfg.Region),
		awsConfig.WithHTTPClient(httpClient),
	}

	// Only set static credentials if both access key and secret key are provided.
	// Otherwise, let the AWS SDK use its default credential chain (environment variables, IAM role, etc.).
	if d.cfg.AccessKey != "" && d.cfg.SecretKey != "" {
		credProvider := credentials.NewStaticCredentialsProvider(d.cfg.AccessKey, string(d.cfg.SecretKey), "")
		configOptions = append(configOptions, awsConfig.WithCredentialsProvider(credProvider))
	}

	// Set the profile if provided.
	if d.cfg.Profile != "" {
		configOptions = append(configOptions, awsConfig.WithSharedConfigProfile(d.cfg.Profile))
	}

	cfg, err := awsConfig.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return nil, fmt.Errorf("could not create aws config: %w", err)
	}

	// If the role ARN is set, assume the role to get credentials and set the credentials provider in the config.
	if d.cfg.RoleARN != "" {
		assumeProvider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), d.cfg.RoleARN)
		cfg.Credentials = aws.NewCredentialsCache(assumeProvider)
	}

	d.lightsail = lightsail.NewFromConfig(cfg)

	return d.lightsail, nil
}

func (d *LightsailDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	lightsailClient, err := d.lightsailClient(ctx)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	input := &lightsail.GetInstancesInput{}

	output, err := lightsailClient.GetInstances(ctx, input)
	if err != nil {
		var awsErr smithy.APIError
		if errors.As(err, &awsErr) && (awsErr.ErrorCode() == "AuthFailure" || awsErr.ErrorCode() == "UnauthorizedOperation") {
			d.lightsail = nil
		}
		return nil, fmt.Errorf("could not get instances: %w", err)
	}

	for _, inst := range output.Instances {
		if inst.PrivateIpAddress == nil {
			continue
		}

		labels := model.LabelSet{
			lightsailLabelAZ:                  model.LabelValue(*inst.Location.AvailabilityZone),
			lightsailLabelBlueprintID:         model.LabelValue(*inst.BlueprintId),
			lightsailLabelBundleID:            model.LabelValue(*inst.BundleId),
			lightsailLabelInstanceName:        model.LabelValue(*inst.Name),
			lightsailLabelInstanceState:       model.LabelValue(*inst.State.Name),
			lightsailLabelInstanceSupportCode: model.LabelValue(*inst.SupportCode),
			lightsailLabelPrivateIP:           model.LabelValue(*inst.PrivateIpAddress),
			lightsailLabelRegion:              model.LabelValue(d.cfg.Region),
		}

		addr := net.JoinHostPort(*inst.PrivateIpAddress, strconv.Itoa(d.cfg.Port))
		labels[model.AddressLabel] = model.LabelValue(addr)

		if inst.PublicIpAddress != nil {
			labels[lightsailLabelPublicIP] = model.LabelValue(*inst.PublicIpAddress)
		}

		if len(inst.Ipv6Addresses) > 0 {
			var ipv6addrs []string
			ipv6addrs = append(ipv6addrs, inst.Ipv6Addresses...)
			labels[lightsailLabelIPv6Addresses] = model.LabelValue(
				lightsailLabelSeparator +
					strings.Join(ipv6addrs, lightsailLabelSeparator) +
					lightsailLabelSeparator)
		}

		for _, t := range inst.Tags {
			if t.Key == nil || t.Value == nil {
				continue
			}
			name := strutil.SanitizeLabelName(*t.Key)
			labels[lightsailLabelTag+model.LabelName(name)] = model.LabelValue(*t.Value)
		}

		tg.Targets = append(tg.Targets, labels)
	}
	return []*targetgroup.Group{tg}, nil
}
