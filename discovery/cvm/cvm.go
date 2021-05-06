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

package cvm

import (
	"context"
	"encoding/base32"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	sts "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sts/v20180813"
)

const (
	cvmLabelPrefix = model.MetaLabelPrefix + "cvm_"

	cvmLabelProjectID     = cvmLabelPrefix + "project_id"
	cvmLabelRegion        = cvmLabelPrefix + "region"
	cvmLabelHostID        = cvmLabelPrefix + "host_id"
	cvmLabelZone          = cvmLabelPrefix + "zone"
	cvmLabelInstanceID    = cvmLabelPrefix + "instance_id"
	cvmLabelInstanceType  = cvmLabelPrefix + "instance_type"
	cvmLabelInstanceState = cvmLabelPrefix + "instance_state"
	cvmLabelPrivateIP     = cvmLabelPrefix + "private_ip"
	cvmLabelPublicIP      = cvmLabelPrefix + "public_ip"
	cvmLabelVPCID         = cvmLabelPrefix + "vpc_id"
	cvmLabelSubnetID      = cvmLabelPrefix + "subnet_id"
	cvmLabelOsName        = cvmLabelPrefix + "os_name"
	cvmLabelTag           = cvmLabelPrefix + "tag_"

	Separator = ","

	DefaultPageLimit = 100
)

// DefaultSDConfig is the default CVM SD configuration.
var DefaultSDConfig = SDConfig{
	Port:            80,
	RefreshInterval: model.Duration(60 * time.Second),
}

// Filter is the configuration for filtering CVM instances.
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// SDConfig is the configuration for TencentCloud CVM based service discovery.
type SDConfig struct {
	Endpoint        string         `yaml:"endpoint,omitempty"`
	Region          string         `yaml:"region,omitempty"`
	Port            int            `yaml:"port"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Filters         []*Filter      `yaml:"filters"`

	// Tencent CVM Auth: https://github.com/tencentcloud/tencentcloud-sdk-go
	SecretID  string             `yaml:"secret_id,omitempty"`
	SecretKey config_util.Secret `yaml:"secret_key,omitempty"`
	Token     config_util.Secret `yaml:"token,omitempty"`
	RoleArn   string             `yaml:"role_arn,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "cvm" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger)
}

// Discovery periodically performs CVM-DescribeInstance requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	conf   *SDConfig
	logger log.Logger
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if c.Region == "" {
		// todo get region from metadata api if region is blank
		return errors.New("cvm_sd_configs requires a region")
	}
	for _, f := range c.Filters {
		if len(f.Values) == 0 {
			return errors.New("cvm_sd_configs filter values cannot be empty")
		}
	}
	return nil
}

// NewDiscovery returns a new cvmDiscovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &Discovery{
		conf:   conf,
		logger: logger,
	}

	d.Discovery = refresh.NewDiscovery(
		logger,
		"cvm",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	level.Debug(d.logger).Log("refresh with sd conf:", d.conf.Region, d.conf.Endpoint, d.conf.Filters)

	client, err := getCvmClient(d.conf)
	if err != nil {
		return nil, errors.Wrap(err, "could not create cvm client")
	}
	tg := &targetgroup.Group{
		Source: d.conf.Region,
	}

	var filters []*cvm.Filter
	for _, f := range d.conf.Filters {
		filters = append(filters, &cvm.Filter{
			Name:   common.StringPtr(f.Name),
			Values: common.StringPtrs(f.Values),
		})
	}
	request := cvm.NewDescribeInstancesRequest()
	request.Filters = filters
	request.Limit = common.Int64Ptr(DefaultPageLimit)

	var offset int64 = 0

	for {
		request.Offset = common.Int64Ptr(offset)

		resp, err := client.DescribeInstances(request)
		if err != nil {
			return nil, errors.Wrap(err, "could not describe cvm instances")
		}

		for _, inst := range resp.Response.InstanceSet {
			if len(inst.PrivateIpAddresses) == 0 {
				continue
			}
			tg.Targets = append(tg.Targets, d.extractLabels(inst))
		}

		if len(resp.Response.InstanceSet) < DefaultPageLimit {
			break
		}
		offset += DefaultPageLimit
	}
	return []*targetgroup.Group{tg}, nil
}

func (d *Discovery) extractLabels(inst *cvm.Instance) model.LabelSet {
	labels := model.LabelSet{
		cvmLabelInstanceID: strPtrToLabelValue(inst.InstanceId),
		cvmLabelRegion:     model.LabelValue(d.conf.Region),
	}

	addr := net.JoinHostPort(*inst.PrivateIpAddresses[0], fmt.Sprintf("%d", d.conf.Port))
	labels[model.AddressLabel] = model.LabelValue(addr)

	if inst.Placement != nil {
		labels[cvmLabelZone] = strPtrToLabelValue(inst.Placement.Zone)
		labels[cvmLabelProjectID] = intPrtToLabelValue(inst.Placement.ProjectId)
		if inst.Placement.HostIds != nil {
			labels[cvmLabelHostID] = strPtrSliceToLabelValue(inst.Placement.HostIds, false)
		}
	}

	labels[cvmLabelInstanceType] = strPtrToLabelValue(inst.InstanceType)
	labels[cvmLabelPrivateIP] = strPtrSliceToLabelValue(inst.PrivateIpAddresses, false)
	labels[cvmLabelPublicIP] = strPtrSliceToLabelValue(inst.PublicIpAddresses, false)
	labels[cvmLabelInstanceState] = strPtrToLabelValue(inst.InstanceState)

	if inst.VirtualPrivateCloud != nil {
		labels[cvmLabelVPCID] = strPtrToLabelValue(inst.VirtualPrivateCloud.VpcId)
		labels[cvmLabelSubnetID] = strPtrToLabelValue(inst.VirtualPrivateCloud.SubnetId)
	}

	for _, tag := range inst.Tags {
		labelName := model.LabelName(*tag.Key)
		labels[cvmLabelTag+labelName] = strPtrToLabelValue(tag.Value)
		// if key is not valid as label name, encode it to keep all information
		if !labelName.IsValid() {
			tagEncoded := strings.ReplaceAll(base32.StdEncoding.EncodeToString([]byte(*tag.Key)), "=", "")
			labels[cvmLabelTag+model.LabelName(tagEncoded)] = strPtrToLabelValue(tag.Value)
		}
	}

	labels[cvmLabelOsName] = strPtrToLabelValue(inst.OsName)
	return labels
}

func getClientConfig(config *SDConfig) (*common.Credential, *profile.ClientProfile) {
	credential := common.NewTokenCredential(
		config.SecretID,
		string(config.SecretKey),
		string(config.Token),
	)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = config.Endpoint

	return credential, cpf
}

func getCvmClient(config *SDConfig) (client *cvm.Client, err error) {
	credential, cpf := getClientConfig(config)
	if config.RoleArn != "" {
		// role_arn mode
		access, err := getUserAccess(config, config.RoleArn)
		if err != nil {
			return nil, err
		}
		credential = common.NewTokenCredential(*access.TmpSecretId, *access.TmpSecretKey, *access.Token)
	}
	return cvm.NewClient(credential, config.Region, cpf)
}

func getUserAccess(config *SDConfig, roleArn string) (*sts.Credentials, error) {
	credential, cpf := getClientConfig(config)
	client, err := sts.NewClient(credential, config.Region, cpf)
	if err != nil {
		return nil, err
	}

	var duration uint64 = 3600
	request := sts.NewAssumeRoleRequest()
	request.RoleArn = &roleArn
	request.DurationSeconds = &duration
	request.RoleSessionName = common.StringPtr(fmt.Sprintf("%d", time.Now().Unix()))
	response, err := client.AssumeRole(request)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Response == nil || response.Response.Credentials == nil {
		return nil, errors.New("cvm_sd get user access fail")
	}
	return response.Response.Credentials, nil
}

func strPtrSliceToLabelValue(slice []*string, takeOne bool) model.LabelValue {
	if slice != nil {
		if takeOne {
			return model.LabelValue(*slice[0])
		}
		return model.LabelValue(strings.Join(common.StringValues(slice), Separator))
	}
	return ""
}

func strPtrToLabelValue(ptr *string) model.LabelValue {
	if ptr != nil {
		return model.LabelValue(*ptr)
	}
	return ""
}

func intPrtToLabelValue(ptr *int64) model.LabelValue {
	if ptr != nil {
		return model.LabelValue(fmt.Sprintf("%d", *ptr))
	}
	return ""
}
