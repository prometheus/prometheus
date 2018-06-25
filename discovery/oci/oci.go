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

package oci

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	"io/ioutil"
)

const (
	ociLabel              = model.MetaLabelPrefix + "oci_"
	ociLabelAZ            = ociLabel + "availability_zone"
	ociLabelInstanceID    = ociLabel + "instance_id"
	ociLabelInstanceState = ociLabel + "instance_state"
	ociLabelInstanceType  = ociLabel + "instance_type"
	ociLabelPublicDNS     = ociLabel + "public_dns_name"
	ociLabelPublicIP      = ociLabel + "public_ip"
	ociLabelPrivateIP     = ociLabel + "private_ip"
	ociLabelSubnetID      = ociLabel + "subnet_id"
	ociLabelTag           = ociLabel + "tag_"
	ociLabelVPCID         = ociLabel + "vpc_id"
	subnetSeparator       = ","
)

var (
	ociSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_oci_refresh_failures_total",
			Help: "The number of OCI-SD scrape failures.",
		})
	ociSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_oci_refresh_duration_seconds",
			Help: "The duration of a OCI-SD refresh in seconds.",
		})
	// DefaultSDConfig is the default OCI SD configuration
	DefaultSDConfig = SDConfig{}
)

// Filter is the configuration for filtering OCI instances.
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// SDConfig is the configuration for OCI based service discovery.
type SDConfig struct {
	User        string `yaml:"user"`
	FingerPrint string `yaml:"fingerprint"`
	KeyFile     string `yaml:"key_file"`
	PassPhrase  string `yaml:"pass_phrase,omitempty"`
	Tenancy     string `yaml:"tenancy"`
	Region      string `yaml:"region"`
	// TODO Guido: consider to add an availability domain and a compartement
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.User == "" {
		return fmt.Errorf("OCI SD configuration requires a user")
	}
	if c.FingerPrint == "" {
		return fmt.Errorf("OCI SD configuration requires a fingerprint")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("OCI SD configuration requires a key file")
	}
	if c.Tenancy == "" {
		return fmt.Errorf("OCI SD configuration requires a tenancy")
	}
	if c.Region == "" {
		return fmt.Errorf("OCI SD configuration requires a region")
	}
	return nil
}

func init() {
	prometheus.MustRegister(ociSDRefreshFailuresCount)
	prometheus.MustRegister(ociSDRefreshDuration)
}

// Discovery periodically performs OCI-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	config *SDConfig
	client core.ComputeClient
	logger log.Logger
}

// NewDiscovery returns a new OCI Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	privateKey, err := loadKey(conf.KeyFile)
	if err != nil {
		return nil, err
	}
	ociConfig := common.NewRawConfigurationProvider(
		conf.Tenancy,
		conf.User,
		conf.Region,
		conf.FingerPrint,
		privateKey,
		&conf.PassPhrase,
	)
	client, err := core.NewComputeClientWithConfigurationProvider(ociConfig)
	if err != nil {
		return nil, err
	}
	return &Discovery{
		config: conf,
		client: client,
		logger: logger,
	}, nil
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

func loadKey(keyFile string) (string, error) {
	data, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	t0 := time.Now()
	defer func() {
		ociSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			ociSDRefreshFailuresCount.Inc()
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

	var filters []*ec2.Filter
	for _, f := range d.filters {
		filters = append(filters, &ec2.Filter{
			Name:   aws.String(f.Name),
			Values: aws.StringSlice(f.Values),
		})
	}

	input := &ec2.DescribeInstancesInput{Filters: filters}

	if err = ec2s.DescribeInstancesPages(input, func(p *ec2.DescribeInstancesOutput, lastPage bool) bool {
		for _, r := range p.Reservations {
			for _, inst := range r.Instances {
				if inst.PrivateIpAddress == nil {
					continue
				}
				labels := model.LabelSet{
					ociLabelInstanceID: model.LabelValue(*inst.InstanceId),
				}
				labels[ociLabelPrivateIP] = model.LabelValue(*inst.PrivateIpAddress)
				addr := net.JoinHostPort(*inst.PrivateIpAddress, fmt.Sprintf("%d", d.port))
				labels[model.AddressLabel] = model.LabelValue(addr)

				if inst.PublicIpAddress != nil {
					labels[ociLabelPublicIP] = model.LabelValue(*inst.PublicIpAddress)
					labels[ociLabelPublicDNS] = model.LabelValue(*inst.PublicDnsName)
				}

				labels[ociLabelAZ] = model.LabelValue(*inst.Placement.AvailabilityZone)
				labels[ociLabelInstanceState] = model.LabelValue(*inst.State.Name)
				labels[ociLabelInstanceType] = model.LabelValue(*inst.InstanceType)

				if inst.VpcId != nil {
					labels[ociLabelVPCID] = model.LabelValue(*inst.VpcId)

					subnetsMap := make(map[string]struct{})
					for _, eni := range inst.NetworkInterfaces {
						subnetsMap[*eni.SubnetId] = struct{}{}
					}
					subnets := []string{}
					for k := range subnetsMap {
						subnets = append(subnets, k)
					}
					labels[ociLabelSubnetID] = model.LabelValue(
						subnetSeparator +
							strings.Join(subnets, subnetSeparator) +
							subnetSeparator)
				}

				for _, t := range inst.Tags {
					if t == nil || t.Key == nil || t.Value == nil {
						continue
					}
					name := strutil.SanitizeLabelName(*t.Key)
					labels[ociLabelTag+model.LabelName(name)] = model.LabelValue(*t.Value)
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
