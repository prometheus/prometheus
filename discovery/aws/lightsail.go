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
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

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
	lightsailLabelTag                 = lightsailLabel + "tag_"
	lightsailLabelSeparator           = ","
)

var (
	// DefaultLightsailSDConfig is the default Lightsail SD configuration.
	DefaultLightsailSDConfig = LightsailSDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
	}
)

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
}

// Name returns the name of the Lightsail Config.
func (*LightsailSDConfig) Name() string { return "lightsail" }

// NewDiscoverer returns a Discoverer for the Lightsail Config.
func (c *LightsailSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewLightsailDiscovery(c, opts.Logger), nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the Lightsail Config.
func (c *LightsailSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultLightsailSDConfig
	type plain LightsailSDConfig
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
			return errors.New("Lightsail SD configuration requires a region")
		}
		c.Region = region
	}
	return nil
}

// LightsailDiscovery periodically performs Lightsail-SD requests. It implements
// the Discoverer interface.
type LightsailDiscovery struct {
	*refresh.Discovery
	cfg       *LightsailSDConfig
	lightsail *lightsail.Lightsail
}

// NewLightsailDiscovery returns a new LightsailDiscovery which periodically refreshes its targets.
func NewLightsailDiscovery(conf *LightsailSDConfig, logger log.Logger) *LightsailDiscovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &LightsailDiscovery{
		cfg: conf,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"lightsail",
		time.Duration(d.cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *LightsailDiscovery) lightsailClient() (*lightsail.Lightsail, error) {
	if d.lightsail != nil {
		return d.lightsail, nil
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
		d.lightsail = lightsail.New(sess, &aws.Config{Credentials: creds})
	} else {
		d.lightsail = lightsail.New(sess)
	}

	return d.lightsail, nil
}

func (d *LightsailDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	lightsailClient, err := d.lightsailClient()
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	input := &lightsail.GetInstancesInput{}

	output, err := lightsailClient.GetInstancesWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && (awsErr.Code() == "AuthFailure" || awsErr.Code() == "UnauthorizedOperation") {
			d.lightsail = nil
		}
		return nil, errors.Wrap(err, "could not get instances")
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
		}

		addr := net.JoinHostPort(*inst.PrivateIpAddress, fmt.Sprintf("%d", d.cfg.Port))
		labels[model.AddressLabel] = model.LabelValue(addr)

		if inst.PublicIpAddress != nil {
			labels[lightsailLabelPublicIP] = model.LabelValue(*inst.PublicIpAddress)
		}

		if len(inst.Ipv6Addresses) > 0 {
			var ipv6addrs []string
			for _, ipv6addr := range inst.Ipv6Addresses {
				ipv6addrs = append(ipv6addrs, *ipv6addr)
			}
			labels[lightsailLabelIPv6Addresses] = model.LabelValue(
				lightsailLabelSeparator +
					strings.Join(ipv6addrs, lightsailLabelSeparator) +
					lightsailLabelSeparator)
		}

		for _, t := range inst.Tags {
			if t == nil || t.Key == nil || t.Value == nil {
				continue
			}
			name := strutil.SanitizeLabelName(*t.Key)
			labels[lightsailLabelTag+model.LabelName(name)] = model.LabelValue(*t.Value)
		}

		tg.Targets = append(tg.Targets, labels)
	}
	return []*targetgroup.Group{tg}, nil
}
