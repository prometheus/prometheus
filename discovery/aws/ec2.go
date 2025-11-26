// Copyright 2021 The Prometheus Authors
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
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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
	ec2Label                     = model.MetaLabelPrefix + "ec2_"
	ec2LabelAMI                  = ec2Label + "ami"
	ec2LabelAZ                   = ec2Label + "availability_zone"
	ec2LabelAZID                 = ec2Label + "availability_zone_id"
	ec2LabelArch                 = ec2Label + "architecture"
	ec2LabelIPv6Addresses        = ec2Label + "ipv6_addresses"
	ec2LabelInstanceID           = ec2Label + "instance_id"
	ec2LabelInstanceLifecycle    = ec2Label + "instance_lifecycle"
	ec2LabelInstanceState        = ec2Label + "instance_state"
	ec2LabelInstanceType         = ec2Label + "instance_type"
	ec2LabelOwnerID              = ec2Label + "owner_id"
	ec2LabelPlatform             = ec2Label + "platform"
	ec2LabelPrimaryIPv6Addresses = ec2Label + "primary_ipv6_addresses"
	ec2LabelPrimarySubnetID      = ec2Label + "primary_subnet_id"
	ec2LabelPrivateDNS           = ec2Label + "private_dns_name"
	ec2LabelPrivateIP            = ec2Label + "private_ip"
	ec2LabelPublicDNS            = ec2Label + "public_dns_name"
	ec2LabelPublicIP             = ec2Label + "public_ip"
	ec2LabelRegion               = ec2Label + "region"
	ec2LabelSubnetID             = ec2Label + "subnet_id"
	ec2LabelTag                  = ec2Label + "tag_"
	ec2LabelVPCID                = ec2Label + "vpc_id"
	ec2LabelSeparator            = ","
)

// DefaultEC2SDConfig is the default EC2 SD configuration.
var DefaultEC2SDConfig = EC2SDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&EC2SDConfig{})
}

// EC2Filter is the configuration for filtering EC2 instances.
type EC2Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// EC2SDConfig is the configuration for EC2 based service discovery.
type EC2SDConfig struct {
	Endpoint        string         `yaml:"endpoint"`
	Region          string         `yaml:"region"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       config.Secret  `yaml:"secret_key,omitempty"`
	Profile         string         `yaml:"profile,omitempty"`
	RoleARN         string         `yaml:"role_arn,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`
	Filters         []*EC2Filter   `yaml:"filters"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*EC2SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &ec2Metrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the EC2 Config.
func (*EC2SDConfig) Name() string { return "ec2" }

// NewDiscoverer returns a Discoverer for the EC2 Config.
func (c *EC2SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewEC2Discovery(c, opts)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the EC2 Config.
func (c *EC2SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultEC2SDConfig
	type plain EC2SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if c.Region == "" {
		cfg, err := awsConfig.LoadDefaultConfig(context.Background())
		if err != nil {
			return err
		}

		if cfg.Region != "" {
			// If the region is already set in the config, use it.
			// This can happen if the user has set the region in the AWS config file or environment variables.
			c.Region = cfg.Region
		}

		if c.Region == "" {
			// Try to get the region from the instance metadata service (IMDS).
			imdsClient := imds.NewFromConfig(cfg)
			region, err := imdsClient.GetRegion(context.Background(), &imds.GetRegionInput{})
			if err != nil {
				return err
			}
			c.Region = region.Region
		}
	}

	if c.Region == "" {
		return errors.New("EC2 SD configuration requires a region")
	}

	for _, f := range c.Filters {
		if len(f.Values) == 0 {
			return errors.New("EC2 SD configuration filter values cannot be empty")
		}
	}
	return c.HTTPClientConfig.Validate()
}

type ec2Client interface {
	DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

// EC2Discovery periodically performs EC2-SD requests. It implements
// the Discoverer interface.
type EC2Discovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *EC2SDConfig
	ec2    ec2Client

	// azToAZID maps this account's availability zones to their underlying AZ
	// ID, e.g. eu-west-2a -> euw2-az2. Refreshes are performed sequentially, so
	// no locking is required.
	azToAZID map[string]string
}

// NewEC2Discovery returns a new EC2Discovery which periodically refreshes its targets.
func NewEC2Discovery(conf *EC2SDConfig, opts discovery.DiscovererOptions) (*EC2Discovery, error) {
	m, ok := opts.Metrics.(*ec2Metrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}
	d := &EC2Discovery{
		logger: opts.Logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "ec2",
			SetName:             opts.SetName,
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *EC2Discovery) ec2Client(ctx context.Context) (ec2Client, error) {
	if d.ec2 != nil {
		return d.ec2, nil
	}

	// Build the HTTP client from the provided HTTPClientConfig.
	httpClient, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "ec2_sd")
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

	d.ec2 = ec2.NewFromConfig(cfg)

	return d.ec2, nil
}

func (d *EC2Discovery) refreshAZIDs(ctx context.Context) error {
	azs, err := d.ec2.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return err
	}
	d.azToAZID = make(map[string]string, len(azs.AvailabilityZones))
	for _, az := range azs.AvailabilityZones {
		d.azToAZID[*az.ZoneName] = *az.ZoneId
	}
	return nil
}

func (d *EC2Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	ec2Client, err := d.ec2Client(ctx)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	var filters []ec2Types.Filter
	for _, f := range d.cfg.Filters {
		filters = append(filters, ec2Types.Filter{
			Name:   aws.String(f.Name),
			Values: f.Values,
		})
	}

	// Only refresh the AZ ID map if we have never been able to build one.
	// Prometheus requires a reload if AWS adds a new AZ to the region.
	if d.azToAZID == nil {
		if err := d.refreshAZIDs(ctx); err != nil {
			d.logger.Debug(
				"Unable to describe availability zones",
				"err", err)
		}
	}

	input := &ec2.DescribeInstancesInput{Filters: filters}
	paginator := ec2.NewDescribeInstancesPaginator(ec2Client, input)

	for paginator.HasMorePages() {
		p, err := paginator.NextPage(ctx)
		if err != nil {
			var awsErr smithy.APIError
			if errors.As(err, &awsErr) && (awsErr.ErrorCode() == "AuthFailure" || awsErr.ErrorCode() == "UnauthorizedOperation") {
				d.ec2 = nil
			}
			return nil, fmt.Errorf("could not describe instances: %w", err)
		}

		for _, r := range p.Reservations {
			for _, inst := range r.Instances {
				if inst.PrivateIpAddress == nil {
					continue
				}

				labels := model.LabelSet{
					ec2LabelInstanceID: model.LabelValue(*inst.InstanceId),
					ec2LabelRegion:     model.LabelValue(d.cfg.Region),
				}

				if r.OwnerId != nil {
					labels[ec2LabelOwnerID] = model.LabelValue(*r.OwnerId)
				}

				labels[ec2LabelPrivateIP] = model.LabelValue(*inst.PrivateIpAddress)
				if inst.PrivateDnsName != nil {
					labels[ec2LabelPrivateDNS] = model.LabelValue(*inst.PrivateDnsName)
				}
				addr := net.JoinHostPort(*inst.PrivateIpAddress, strconv.Itoa(d.cfg.Port))
				labels[model.AddressLabel] = model.LabelValue(addr)

				if inst.Platform != "" {
					labels[ec2LabelPlatform] = model.LabelValue(inst.Platform)
				}

				if inst.PublicIpAddress != nil {
					labels[ec2LabelPublicIP] = model.LabelValue(*inst.PublicIpAddress)
					labels[ec2LabelPublicDNS] = model.LabelValue(*inst.PublicDnsName)
				}
				labels[ec2LabelAMI] = model.LabelValue(*inst.ImageId)
				labels[ec2LabelAZ] = model.LabelValue(*inst.Placement.AvailabilityZone)
				azID, ok := d.azToAZID[*inst.Placement.AvailabilityZone]
				if !ok && d.azToAZID != nil {
					d.logger.Debug(
						"Availability zone ID not found",
						"az", *inst.Placement.AvailabilityZone)
				}
				labels[ec2LabelAZID] = model.LabelValue(azID)
				labels[ec2LabelInstanceState] = model.LabelValue(inst.State.Name)
				labels[ec2LabelInstanceType] = model.LabelValue(inst.InstanceType)

				if inst.InstanceLifecycle != "" {
					labels[ec2LabelInstanceLifecycle] = model.LabelValue(inst.InstanceLifecycle)
				}

				if inst.Architecture != "" {
					labels[ec2LabelArch] = model.LabelValue(inst.Architecture)
				}

				if inst.VpcId != nil {
					labels[ec2LabelVPCID] = model.LabelValue(*inst.VpcId)
					labels[ec2LabelPrimarySubnetID] = model.LabelValue(*inst.SubnetId)

					var subnets []string
					var ipv6addrs []string
					var primaryipv6addrs []string
					subnetsMap := make(map[string]struct{})
					for _, eni := range inst.NetworkInterfaces {
						if eni.SubnetId == nil {
							continue
						}
						// Deduplicate VPC Subnet IDs maintaining the order of the subnets returned by EC2.
						if _, ok := subnetsMap[*eni.SubnetId]; !ok {
							subnetsMap[*eni.SubnetId] = struct{}{}
							subnets = append(subnets, *eni.SubnetId)
						}

						for _, ipv6addr := range eni.Ipv6Addresses {
							ipv6addrs = append(ipv6addrs, *ipv6addr.Ipv6Address)
							if *ipv6addr.IsPrimaryIpv6 {
								// we might have to extend the slice with more than one element
								// that could leave empty strings in the list which is intentional
								// to keep the position/device index information
								for int32(len(primaryipv6addrs)) <= *eni.Attachment.DeviceIndex {
									primaryipv6addrs = append(primaryipv6addrs, "")
								}
								primaryipv6addrs[*eni.Attachment.DeviceIndex] = *ipv6addr.Ipv6Address
							}
						}
					}
					labels[ec2LabelSubnetID] = model.LabelValue(
						ec2LabelSeparator +
							strings.Join(subnets, ec2LabelSeparator) +
							ec2LabelSeparator)
					if len(ipv6addrs) > 0 {
						labels[ec2LabelIPv6Addresses] = model.LabelValue(
							ec2LabelSeparator +
								strings.Join(ipv6addrs, ec2LabelSeparator) +
								ec2LabelSeparator)
					}
					if len(primaryipv6addrs) > 0 {
						labels[ec2LabelPrimaryIPv6Addresses] = model.LabelValue(
							ec2LabelSeparator +
								strings.Join(primaryipv6addrs, ec2LabelSeparator) +
								ec2LabelSeparator)
					}
				}

				for _, t := range inst.Tags {
					if t.Key == nil || t.Value == nil {
						continue
					}
					name := strutil.SanitizeLabelName(*t.Key)
					labels[ec2LabelTag+model.LabelName(name)] = model.LabelValue(*t.Value)
				}
				tg.Targets = append(tg.Targets, labels)
			}
		}
	}

	return []*targetgroup.Group{tg}, nil
}
