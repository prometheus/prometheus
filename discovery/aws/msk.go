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
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kafka"
	"github.com/go-kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	mskLabel             = model.MetaLabelPrefix + "msk_"
	mskLabelClusterName  = mskLabel + "cluster_name"
	mskLabelTag          = mskLabel + "tag_"
	mskLabelKafkaVersion = mskLabel + "kafka_version"
	mskLabelClusterSize  = mskLabel + "cluster_size"
	mskLabelSeparator    = ","
)

// DefaultMSKSDConfig is the default MSK SD configuration.
var DefaultMSKSDConfig = MSKSDConfig{
	RefreshInterval: model.Duration(60 * time.Second),
}

func init() {
	discovery.RegisterConfig(&MSKSDConfig{})
}

// MSKSDConfig is the configuration for MSK based service discovery.
type MSKSDConfig struct {
	Region            string         `yaml:"region"`
	AccessKey         string         `yaml:"access_key,omitempty"`
	SecretKey         config.Secret  `yaml:"secret_key,omitempty"`
	Profile           string         `yaml:"profile,omitempty"`
	RoleARN           string         `yaml:"role_arn,omitempty"`
	RefreshInterval   model.Duration `yaml:"refresh_interval,omitempty"`
	ClusterNameFilter string         `yaml:"cluster_name_filter,omitempty"`
}

// Name returns the name of the MSK Config.
func (*MSKSDConfig) Name() string { return "msk" }

// NewDiscoverer returns a Discoverer for the MSK Config.
func (c *MSKSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewMSKDiscovery(c, opts.Logger), nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the MSK Config.
func (c *MSKSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultMSKSDConfig
	type plain MSKSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	return nil
}

// MSKDiscovery periodically performs MSK-SD requests. It implements
// the Discoverer interface.
type MSKDiscovery struct {
	*refresh.Discovery
	logger log.Logger
	cfg    *MSKSDConfig
	kafka  *kafka.Kafka
}

// NewMSKDiscovery returns a new MSKDiscovery which periodically refreshes its targets.
func NewMSKDiscovery(conf *MSKSDConfig, logger log.Logger) *MSKDiscovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &MSKDiscovery{
		logger: logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"MSK",
		time.Duration(d.cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *MSKDiscovery) mskClient(ctx context.Context) (*kafka.Kafka, error) {
	if d.kafka != nil {
		return d.kafka, nil
	}

	creds := credentials.NewStaticCredentials(d.cfg.AccessKey, string(d.cfg.SecretKey), "")
	if d.cfg.AccessKey == "" && d.cfg.SecretKey == "" {
		creds = nil
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
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
		d.kafka = kafka.New(sess, &aws.Config{Credentials: creds})
	} else {
		d.kafka = kafka.New(sess)
	}

	return d.kafka, nil
}

func (d *MSKDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	mskClient, err := d.mskClient(ctx)
	if err != nil {
		return nil, err
	}

	var clustersArn []*string
	input := &kafka.ListClustersInput{}
	if d.cfg.ClusterNameFilter != "" {
		input.SetClusterNameFilter(d.cfg.ClusterNameFilter)
	}

	for {
		result, err := mskClient.ListClusters(input)
		if err != nil {
			return nil, err
		}
		if result == nil {
			break
		}

		var clustersArnPaged []*string
		for _, c := range result.ClusterInfoList {
			clustersArnPaged = append(clustersArnPaged, c.ClusterArn)
		}
		clustersArn = append(clustersArn, clustersArnPaged...)
		input.NextToken = result.NextToken

		if result.NextToken == nil {
			break
		}
	}

	exporterPorts := map[string]int{
		"jmx":  11001,
		"node": 11002,
	}

	var tgs []*targetgroup.Group

	for exporter, port := range exporterPorts {
		tg := &targetgroup.Group{
			Source: d.cfg.Region,
			Labels: model.LabelSet{
				"exporter": model.LabelValue(exporter),
			},
		}

		for _, clusterArn := range clustersArn {
			getBrokersInput := &kafka.GetBootstrapBrokersInput{
				ClusterArn: clusterArn,
			}
			brokers, err := mskClient.GetBootstrapBrokers(getBrokersInput)
			if err != nil {
				continue
			}

			descClusterInput := &kafka.DescribeClusterInput{
				ClusterArn: clusterArn,
			}

			cluster, err := mskClient.DescribeCluster(descClusterInput)

			for _, brokerURL := range strings.Split(*brokers.BootstrapBrokerString, mskLabelSeparator) {
				host, _, _ := net.SplitHostPort(brokerURL)
				addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
				target := model.LabelSet{
					model.AddressLabel: model.LabelValue(addr),
				}
				if err == nil {
					target[mskLabelClusterName] = model.LabelValue(*cluster.ClusterInfo.ClusterName)
					for k, v := range cluster.ClusterInfo.Tags {
						if k == "" || v == nil {
							continue
						}
						name := strutil.SanitizeLabelName(k)
						target[mskLabelTag+model.LabelName(name)] = model.LabelValue(*v)
					}
					target[mskLabelKafkaVersion] = model.LabelValue(*cluster.ClusterInfo.CurrentVersion)
					target[mskLabelClusterSize] = model.LabelValue(fmt.Sprint(*cluster.ClusterInfo.NumberOfBrokerNodes))
				}
				tg.Targets = append(tg.Targets, target)
			}
		}
		tgs = append(tgs, tg)
	}

	return tgs, nil
}
