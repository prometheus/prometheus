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

package discovery

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	ec2Label           = model.MetaLabelPrefix + "ec2_"
	ec2LabelAZ         = ec2Label + "availability_zone"
	ec2LabelInstanceID = ec2Label + "instance_id"
	ec2LabelPublicDNS  = ec2Label + "public_dns_name"
	ec2LabelPublicIP   = ec2Label + "public_ip"
	ec2LabelPrivateIP  = ec2Label + "private_ip"
	ec2LabelSubnetID   = ec2Label + "subnet_id"
	ec2LabelTag        = ec2Label + "tag_"
	ec2LabelVPCID      = ec2Label + "vpc_id"
	subnetSeparator    = ","
)

// EC2Discovery periodically performs EC2-SD requests. It implements
// the TargetProvider interface.
type EC2Discovery struct {
	aws      *aws.Config
	interval time.Duration
	port     int
}

// NewEC2Discovery returns a new EC2Discovery which periodically refreshes its targets.
func NewEC2Discovery(conf *config.EC2SDConfig) *EC2Discovery {
	creds := credentials.NewStaticCredentials(conf.AccessKey, conf.SecretKey, "")
	if conf.AccessKey == "" && conf.SecretKey == "" {
		creds = ec2rolecreds.NewCredentials(session.New())
	}
	return &EC2Discovery{
		aws: &aws.Config{
			Region:      &conf.Region,
			Credentials: creds,
		},
		interval: time.Duration(conf.RefreshInterval),
		port:     conf.Port,
	}
}

// Run implements the TargetProvider interface.
func (ed *EC2Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	ticker := time.NewTicker(ed.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := ed.refresh()
	if err != nil {
		log.Error(err)
	} else {
		ch <- []*config.TargetGroup{tg}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := ed.refresh()
			if err != nil {
				log.Error(err)
			} else {
				ch <- []*config.TargetGroup{tg}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (ed *EC2Discovery) refresh() (*config.TargetGroup, error) {
	ec2s := ec2.New(session.New(), ed.aws)
	tg := &config.TargetGroup{
		Source: *ed.aws.Region,
	}
	if err := ec2s.DescribeInstancesPages(nil, func(p *ec2.DescribeInstancesOutput, lastPage bool) bool {
		for _, r := range p.Reservations {
			for _, inst := range r.Instances {
				if inst.PrivateIpAddress == nil {
					continue
				}
				labels := model.LabelSet{
					ec2LabelInstanceID: model.LabelValue(*inst.InstanceId),
				}
				labels[ec2LabelPrivateIP] = model.LabelValue(*inst.PrivateIpAddress)
				addr := fmt.Sprintf("%s:%d", *inst.PrivateIpAddress, ed.port)
				labels[model.AddressLabel] = model.LabelValue(addr)

				if inst.PublicIpAddress != nil {
					labels[ec2LabelPublicIP] = model.LabelValue(*inst.PublicIpAddress)
					labels[ec2LabelPublicDNS] = model.LabelValue(*inst.PublicDnsName)
				}

				labels[ec2LabelAZ] = model.LabelValue(*inst.Placement.AvailabilityZone)

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
