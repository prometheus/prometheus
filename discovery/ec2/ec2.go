// Copyright 2015 The Prometheus Authors
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

package ec2

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/aws/aws-sdk-go/service/ec2"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	yaml_util "github.com/prometheus/prometheus/util/yaml"
)

const (
	ec2Label              = model.MetaLabelPrefix + "ec2_"
	ec2LabelAZ            = ec2Label + "availability_zone"
	ec2LabelInstanceID    = ec2Label + "instance_id"
	ec2LabelInstanceState = ec2Label + "instance_state"
	ec2LabelInstanceType  = ec2Label + "instance_type"
	ec2LabelPublicDNS     = ec2Label + "public_dns_name"
	ec2LabelPublicIP      = ec2Label + "public_ip"
	ec2LabelPrivateIP     = ec2Label + "private_ip"
	ec2LabelSubnetID      = ec2Label + "subnet_id"
	ec2LabelTag           = ec2Label + "tag_"
	ec2LabelVPCID         = ec2Label + "vpc_id"
	subnetSeparator       = ","
)

var (
	ec2SDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_ec2_refresh_failures_total",
			Help: "The number of EC2-SD scrape failures.",
		})
	ec2SDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_ec2_refresh_duration_seconds",
			Help: "The duration of a EC2-SD refresh in seconds.",
		})
	// DefaultSDConfig is the default EC2 SD configuration.
	DefaultSDConfig = SDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
	}
)

// SDConfig is the configuration for EC2 based service discovery.
type SDConfig struct {
	Region          string             `yaml:"region"`
	AccessKey       string             `yaml:"access_key,omitempty"`
	SecretKey       config_util.Secret `yaml:"secret_key,omitempty"`
	Profile         string             `yaml:"profile,omitempty"`
	RoleARN         string             `yaml:"role_arn,omitempty"`
	RefreshInterval model.Duration     `yaml:"refresh_interval,omitempty"`
	Port            int                `yaml:"port"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := yaml_util.CheckOverflow(c.XXX, "ec2_sd_config"); err != nil {
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
			return fmt.Errorf("EC2 SD configuration requires a region")
		}
		c.Region = region
	}
	return nil
}

func init() {
	prometheus.MustRegister(ec2SDRefreshFailuresCount)
	prometheus.MustRegister(ec2SDRefreshDuration)
}

// Discovery periodically performs EC2-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	aws      *aws.Config
	interval time.Duration
	profile  string
	roleARN  string
	port     int
	logger   log.Logger
}

// NewDiscovery returns a new EC2Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	creds := credentials.NewStaticCredentials(conf.AccessKey, string(conf.SecretKey), "")
	if conf.AccessKey == "" && conf.SecretKey == "" {
		creds = nil
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &Discovery{
		aws: &aws.Config{
			Region:      &conf.Region,
			Credentials: creds,
		},
		profile:  conf.Profile,
		roleARN:  conf.RoleARN,
		interval: time.Duration(conf.RefreshInterval),
		port:     conf.Port,
		logger:   logger,
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
	} else {
		select {
		case ch <- []*targetgroup.Group{tg}:
		case <-ctx.Done():
			return
		}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
				continue
			}

			select {
			case ch <- []*targetgroup.Group{tg}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	t0 := time.Now()
	defer func() {
		ec2SDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			ec2SDRefreshFailuresCount.Inc()
		}
	}()

	sess, err := session.NewSessionWithOptions(session.Options{
		Config:  *d.aws,
		Profile: d.profile,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create aws session: %s", err)
	}

	var ec2s *ec2.EC2
	if d.roleARN != "" {
		creds := stscreds.NewCredentials(sess, d.roleARN)
		ec2s = ec2.New(sess, &aws.Config{Credentials: creds})
	} else {
		ec2s = ec2.New(sess)
	}
	tg = &targetgroup.Group{
		Source: *d.aws.Region,
	}
	if err = ec2s.DescribeInstancesPages(nil, func(p *ec2.DescribeInstancesOutput, lastPage bool) bool {
		for _, r := range p.Reservations {
			for _, inst := range r.Instances {
				if inst.PrivateIpAddress == nil {
					continue
				}
				labels := model.LabelSet{
					ec2LabelInstanceID: model.LabelValue(*inst.InstanceId),
				}
				labels[ec2LabelPrivateIP] = model.LabelValue(*inst.PrivateIpAddress)
				addr := net.JoinHostPort(*inst.PrivateIpAddress, fmt.Sprintf("%d", d.port))
				labels[model.AddressLabel] = model.LabelValue(addr)

				if inst.PublicIpAddress != nil {
					labels[ec2LabelPublicIP] = model.LabelValue(*inst.PublicIpAddress)
					labels[ec2LabelPublicDNS] = model.LabelValue(*inst.PublicDnsName)
				}

				labels[ec2LabelAZ] = model.LabelValue(*inst.Placement.AvailabilityZone)
				labels[ec2LabelInstanceState] = model.LabelValue(*inst.State.Name)
				labels[ec2LabelInstanceType] = model.LabelValue(*inst.InstanceType)

				if inst.VpcId != nil {
					labels[ec2LabelVPCID] = model.LabelValue(*inst.VpcId)

					subnetsMap := make(map[string]struct{})
					for _, eni := range inst.NetworkInterfaces {
						subnetsMap[*eni.SubnetId] = struct{}{}
					}
					subnets := []string{}
					for k := range subnetsMap {
						subnets = append(subnets, k)
					}
					labels[ec2LabelSubnetID] = model.LabelValue(
						subnetSeparator +
							strings.Join(subnets, subnetSeparator) +
							subnetSeparator)
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
		return nil, fmt.Errorf("could not describe instances: %s", err)
	}
	return tg, nil
}
