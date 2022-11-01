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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/go-kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	rdsLabel              = model.MetaLabelPrefix + "rds_"
	rdsLabelARN           = rdsLabel + "arn"
	rdsLabelAZ            = rdsLabel + "availability_zone"
	rdsLabelCluster       = rdsLabel + "cluster"
	rdsLabelEndpoint      = rdsLabel + "endpoint"
	rdsLabelEngine        = rdsLabel + "engine"
	rdsLabelInstanceID    = rdsLabel + "instance_id"
	rdsLabelInstanceClass = rdsLabel + "instance_class"
	rdsLabelInstanceState = rdsLabel + "instance_state"
	rdsLabelSubnetGroup   = rdsLabel + "subnet_id"
	rdsLabelRegion        = rdsLabel + "region"
	rdsLabelTag           = rdsLabel + "tag_"
	rdsLabelVPCID         = rdsLabel + "vpc_id"
	rdsLabelSeparator     = ","
)

// DefaultRDSSDConfig is the default RDS SD configuration.
var DefaultRDSSDConfig = RDSSDConfig{
	Port:            80,
	RefreshInterval: model.Duration(60 * time.Second),
}

func init() {
	discovery.RegisterConfig(&RDSSDConfig{})
}

// RDSFilter is the configuration for filtering RDS instances.
type RDSFilter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// RDSSDConfig is the configuration for RDS based service discovery.
type RDSSDConfig struct {
	Endpoint        string         `yaml:"endpoint"`
	Region          string         `yaml:"region"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       config.Secret  `yaml:"secret_key,omitempty"`
	Profile         string         `yaml:"profile,omitempty"`
	RoleARN         string         `yaml:"role_arn,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`
	Filters         []*RDSFilter   `yaml:"filters"`
}

// Name returns the name of the RDS Config.
func (*RDSSDConfig) Name() string { return "rds" }

// NewDiscoverer returns a Discoverer for the RDS Config.
func (c *RDSSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewRDSDiscovery(c, opts.Logger), nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the RDS Config.
func (c *RDSSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultRDSSDConfig
	type plain RDSSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Region == "" {
		sess, err := session.NewSession()
		if err != nil {
			return err
		}
		metadata := rds.New(sess)
		region := metadata.SigningRegion
		// if err != nil {
		// 	return errors.New("RDS SD configuration requires a region")
		// }
		c.Region = region
	}
	for _, f := range c.Filters {
		if len(f.Values) == 0 {
			return errors.New("RDS SD configuration filter values cannot be empty")
		}
	}
	return nil
}

// RDSDiscovery periodically performs RDS-SD requests. It implements
// the Discoverer interface.
type RDSDiscovery struct {
	*refresh.Discovery
	logger log.Logger
	cfg    *RDSSDConfig
	rds    *rds.RDS
}

// NewRDSDiscovery returns a new RDSDiscovery which periodically refreshes its targets.
func NewRDSDiscovery(conf *RDSSDConfig, logger log.Logger) *RDSDiscovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &RDSDiscovery{
		logger: logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"rds",
		time.Duration(d.cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *RDSDiscovery) rdsClient(ctx context.Context) (*rds.RDS, error) {
	if d.rds != nil {
		return d.rds, nil
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
		return nil, fmt.Errorf("could not create aws session: %w", err)
	}

	if d.cfg.RoleARN != "" {
		creds := stscreds.NewCredentials(sess, d.cfg.RoleARN)
		d.rds = rds.New(sess, &aws.Config{Credentials: creds})
	} else {
		d.rds = rds.New(sess)
	}

	return d.rds, nil
}

func (d *RDSDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	rdsClient, err := d.rdsClient(ctx)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	var filters []*rds.Filter
	for _, f := range d.cfg.Filters {
		filters = append(filters, &rds.Filter{
			Name:   aws.String(f.Name),
			Values: aws.StringSlice(f.Values),
		})
	}

	input := &rds.DescribeDBInstancesInput{Filters: filters}
	if err := rdsClient.DescribeDBInstancesPagesWithContext(ctx, input, func(p *rds.DescribeDBInstancesOutput, lastPage bool) bool {
		for _, inst := range p.DBInstances {
			if inst.DBInstanceArn == nil {
				continue
			}

			labels := model.LabelSet{
				rdsLabelInstanceID: model.LabelValue(*inst.DbiResourceId),
				rdsLabelRegion:     model.LabelValue(d.cfg.Region),
			}

			labels[rdsLabelARN] = model.LabelValue(*inst.DBInstanceArn)
			if inst.DBSubnetGroup != nil {
				labels[rdsLabelSubnetGroup] = model.LabelValue(*inst.DBSubnetGroup.DBSubnetGroupName)
				labels[rdsLabelVPCID] = model.LabelValue(*inst.DBSubnetGroup.VpcId)
			}

			labels[rdsLabelEngine] = model.LabelValue(*inst.Engine)
			labels[rdsLabelAZ] = model.LabelValue(*inst.AvailabilityZone)
			labels[rdsLabelInstanceClass] = model.LabelValue(*inst.DBInstanceClass)
			labels[rdsLabelCluster] = model.LabelValue(*inst.DBClusterIdentifier)

			if inst.Endpoint != nil {
				endpoint := inst.Endpoint.String()
				labels[rdsLabelEndpoint] = model.LabelValue(endpoint)
			}

			labels[rdsLabelInstanceState] = model.LabelValue(*inst.DBInstanceStatus)

			for _, t := range inst.TagList {
				if t == nil || t.Key == nil || t.Value == nil {
					continue
				}
				name := strutil.SanitizeLabelName(*t.Key)
				labels[rdsLabelTag+model.LabelName(name)] = model.LabelValue(*t.Value)
			}
			tg.Targets = append(tg.Targets, labels)
		}
		return true
	}); err != nil {
		var awsErr awserr.Error
		if errors.As(err, &awsErr) && (awsErr.Code() == "AuthFailure" || awsErr.Code() == "UnauthorizedOperation") {
			d.rds = nil
		}
		return nil, fmt.Errorf("could not describe instances: %w", err)
	}
	return []*targetgroup.Group{tg}, nil
}
