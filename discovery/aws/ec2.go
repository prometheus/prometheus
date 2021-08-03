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
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	ec2Label                  = model.MetaLabelPrefix + "ec2_"
	ec2LabelAMI               = ec2Label + "ami"
	ec2LabelAZ                = ec2Label + "availability_zone"
	ec2LabelAZID              = ec2Label + "availability_zone_id"
	ec2LabelArch              = ec2Label + "architecture"
	ec2LabelIPv6Addresses     = ec2Label + "ipv6_addresses"
	ec2LabelInstanceID        = ec2Label + "instance_id"
	ec2LabelInstanceLifecycle = ec2Label + "instance_lifecycle"
	ec2LabelInstanceState     = ec2Label + "instance_state"
	ec2LabelInstanceType      = ec2Label + "instance_type"
	ec2LabelOwnerID           = ec2Label + "owner_id"
	ec2LabelPlatform          = ec2Label + "platform"
	ec2LabelPrimarySubnetID   = ec2Label + "primary_subnet_id"
	ec2LabelPrivateDNS        = ec2Label + "private_dns_name"
	ec2LabelPrivateIP         = ec2Label + "private_ip"
	ec2LabelPublicDNS         = ec2Label + "public_dns_name"
	ec2LabelPublicIP          = ec2Label + "public_ip"
	ec2LabelSubnetID          = ec2Label + "subnet_id"
	ec2LabelTag               = ec2Label + "tag_"
	ec2LabelVPCID             = ec2Label + "vpc_id"
	ec2LabelSeparator         = ","
)

var (
	// DefaultEC2SDConfig is the default EC2 SD configuration.
	DefaultEC2SDConfig = EC2SDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
	}
)

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
}

// Name returns the name of the EC2 Config.
func (*EC2SDConfig) Name() string { return "ec2" }

// NewDiscoverer returns a Discoverer for the EC2 Config.
func (c *EC2SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewEC2Discovery(c, opts.Logger), nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the EC2 Config.
func (c *EC2SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultEC2SDConfig
	type plain EC2SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Region == "" {
		sess, err := session.NewSession()
		if err != nil {
			return err
		}
		metadata := ec2metadata.New(sess)
		region, err := metadata.Region()
		if err != nil {
			return errors.New("EC2 SD configuration requires a region")
		}
		c.Region = region
	}
	for _, f := range c.Filters {
		if len(f.Values) == 0 {
			return errors.New("EC2 SD configuration filter values cannot be empty")
		}
	}
	return nil
}

// EC2Discovery periodically performs EC2-SD requests. It implements
// the Discoverer interface.
type EC2Discovery struct {
	*refresh.Discovery
	logger log.Logger
	cfg    *EC2SDConfig
	ec2    *ec2.EC2

	// azToAZID maps this account's availability zones to their underlying AZ
	// ID, e.g. eu-west-2a -> euw2-az2. Refreshes are performed sequentially, so
	// no locking is required.
	azToAZID map[string]string
}

// NewEC2Discovery returns a new EC2Discovery which periodically refreshes its targets.
func NewEC2Discovery(conf *EC2SDConfig, logger log.Logger) *EC2Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &EC2Discovery{
		logger: logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"ec2",
		time.Duration(d.cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *EC2Discovery) ec2Client(ctx context.Context) (*ec2.EC2, error) {
	if d.ec2 != nil {
		return d.ec2, nil
	}

	creds := credentials.NewStaticCredentials(d.cfg.AccessKey, string(d.cfg.SecretKey), "")
	if d.cfg.AccessKey == "" && d.cfg.SecretKey == "" {
		creds = nil
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Endpoint:    &d.cfg.Endpoint,
			Region:      &d.cfg.Region,
			Credentials: creds,
		},
		Profile: d.cfg.Profile,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create aws session")
	}

	if d.cfg.RoleARN != "" {
		creds := stscreds.NewCredentials(sess, d.cfg.RoleARN)
		d.ec2 = ec2.New(sess, &aws.Config{Credentials: creds})
	} else {
		d.ec2 = ec2.New(sess)
	}

	return d.ec2, nil
}

func (d *EC2Discovery) refreshAZIDs(ctx context.Context) error {
	azs, err := d.ec2.DescribeAvailabilityZonesWithContext(ctx, &ec2.DescribeAvailabilityZonesInput{})
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

	var filters []*ec2.Filter
	for _, f := range d.cfg.Filters {
		filters = append(filters, &ec2.Filter{
			Name:   aws.String(f.Name),
			Values: aws.StringSlice(f.Values),
		})
	}

	// Only refresh the AZ ID map if we have never been able to build one.
	// Prometheus requires a reload if AWS adds a new AZ to the region.
	if d.azToAZID == nil {
		if err := d.refreshAZIDs(ctx); err != nil {
			level.Debug(d.logger).Log(
				"msg", "Unable to describe availability zones",
				"err", err)
		}
	}

	input := &ec2.DescribeInstancesInput{Filters: filters}
	if err := ec2Client.DescribeInstancesPagesWithContext(ctx, input, func(p *ec2.DescribeInstancesOutput, lastPage bool) bool {
		for _, r := range p.Reservations {
			for _, inst := range r.Instances {
				if inst.PrivateIpAddress == nil {
					continue
				}

				labels := model.LabelSet{
					ec2LabelInstanceID: model.LabelValue(*inst.InstanceId),
				}

				if r.OwnerId != nil {
					labels[ec2LabelOwnerID] = model.LabelValue(*r.OwnerId)
				}

				labels[ec2LabelPrivateIP] = model.LabelValue(*inst.PrivateIpAddress)
				if inst.PrivateDnsName != nil {
					labels[ec2LabelPrivateDNS] = model.LabelValue(*inst.PrivateDnsName)
				}
				addr := net.JoinHostPort(*inst.PrivateIpAddress, fmt.Sprintf("%d", d.cfg.Port))
				labels[model.AddressLabel] = model.LabelValue(addr)

				if inst.Platform != nil {
					labels[ec2LabelPlatform] = model.LabelValue(*inst.Platform)
				}

				if inst.PublicIpAddress != nil {
					labels[ec2LabelPublicIP] = model.LabelValue(*inst.PublicIpAddress)
					labels[ec2LabelPublicDNS] = model.LabelValue(*inst.PublicDnsName)
				}
				labels[ec2LabelAMI] = model.LabelValue(*inst.ImageId)
				labels[ec2LabelAZ] = model.LabelValue(*inst.Placement.AvailabilityZone)
				azID, ok := d.azToAZID[*inst.Placement.AvailabilityZone]
				if !ok && d.azToAZID != nil {
					level.Debug(d.logger).Log(
						"msg", "Availability zone ID not found",
						"az", *inst.Placement.AvailabilityZone)
				}
				labels[ec2LabelAZID] = model.LabelValue(azID)
				labels[ec2LabelInstanceState] = model.LabelValue(*inst.State.Name)
				labels[ec2LabelInstanceType] = model.LabelValue(*inst.InstanceType)

				if inst.InstanceLifecycle != nil {
					labels[ec2LabelInstanceLifecycle] = model.LabelValue(*inst.InstanceLifecycle)
				}

				if inst.Architecture != nil {
					labels[ec2LabelArch] = model.LabelValue(*inst.Architecture)
				}

				if inst.VpcId != nil {
					labels[ec2LabelVPCID] = model.LabelValue(*inst.VpcId)
					labels[ec2LabelPrimarySubnetID] = model.LabelValue(*inst.SubnetId)

					var subnets []string
					var ipv6addrs []string
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
				}

				for _, t := range inst.Tags {
					if t == nil || t.Key == nil || t.Value == nil {
						continue
					}
					name := strutil.SanitizeLabelName(*t.Key)
					labels[ec2LabelTag+model.LabelName(name)] = model.LabelValue(*t.Value)
				}
				tg.Targets = append(tg.Targets, labels)
			}
		}
		return true
	}); err != nil {
		if awsErr, ok := err.(awserr.Error); ok && (awsErr.Code() == "AuthFailure" || awsErr.Code() == "UnauthorizedOperation") {
			d.ec2 = nil
		}
		return nil, errors.Wrap(err, "could not describe instances")
	}
	return []*targetgroup.Group{tg}, nil
}
