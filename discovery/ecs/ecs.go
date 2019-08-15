package ecs

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"net"
	"time"

	ecs_pop "github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
)

const (
	ecsLabel = model.MetaLabelPrefix + "ecs_"

	ecsLabelPublicIp  = ecsLabel + "public_ip"  // classic public ip
	ecsLabelInnerIp   = ecsLabel + "inner_ip"   // classic inner ip
	ecsLabelEip       = ecsLabel + "eip"        // vpc public eip
	ecsLabelPrivateIp = ecsLabel + "private_ip" // vpc private ip

	ecsLabelInstanceId  = ecsLabel + "instance_id"
	ecsLabelRegionId    = ecsLabel + "region_id"
	ecsLabelStatus      = ecsLabel + "status"
	ecsLabelZoneId      = ecsLabel + "zone_id"
	ecsLabelNetworkType = ecsLabel + "network_type"
	ecsLabelTag         = ecsLabel + "tag_"
)

// SDConfig is the configuration for Azure based service discovery.
type SDConfig struct {
	Port            int            `yaml:"port"`
	UserId          string         `yaml:"user_id,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	RegionId        string         `yaml:"region_id,omitempty"`

	// Alibaba ECS Auth Args
	// https://github.com/aliyun/alibaba-cloud-sdk-go/blob/master/docs/2-Client-EN.md
	AccessKey         string `yaml:"access_key,omitempty"`
	AccessKeySecret   string `yaml:"access_key_secret,omitempty"`
	StsToken          string `yaml:"sts_token,omitempty"`
	RoleArn           string `yaml:"role_arn,omitempty"`
	RoleSessionName   string `yaml:"role_session_name,omitempty"`
	Policy            string `yaml:"policy,omitempty"`
	RoleName          string `yaml:"role_name,omitempty"`
	PublicKeyId       string `yaml:"public_key_id,omitempty"`
	PrivateKey        string `yaml:"private_key,omitempty"`
	SessionExpiration int    `yaml:"session_expiration,omitempty"`
}

type Discovery struct {
	*refresh.Discovery
	logger log.Logger
	ecsCfg *SDConfig
	port   int
}

// NewDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &Discovery{
		ecsCfg: cfg,
		port:   cfg.Port,
		logger: logger,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"ecs",
		time.Duration(cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {

	defer level.Debug(d.logger).Log("msg", "ECS discovery completed")

	describeInstancesRequest := ecs_pop.CreateDescribeInstancesRequest()
	describeInstancesRequest.RegionId = "cn-hangzhou"

	client, clientErr := getEcsClient(d.ecsCfg)
	if clientErr != nil {
		return nil, errors.Wrap(clientErr, "could not create alibaba ecs client.")
	}

	describeInstancesResponse, responseErr := client.DescribeInstances(describeInstancesRequest)
	if responseErr != nil {
		return nil, errors.Wrap(responseErr, "could not get ecs describeInstances response.")
	}

	level.Debug(d.logger).Log("msg", "Found Instances during ECS discovery.", "count", len(describeInstancesResponse.Instances.Instance))

	tg := &targetgroup.Group{
		Source: d.ecsCfg.RegionId,
	}

	for _, instance := range describeInstancesResponse.Instances.Instance {

		labels := model.LabelSet{
			ecsLabelInstanceId:  model.LabelValue(instance.InstanceId),
			ecsLabelRegionId:    model.LabelValue(instance.RegionId),
			ecsLabelStatus:      model.LabelValue(instance.Status),
			ecsLabelZoneId:      model.LabelValue(instance.ZoneId),
			ecsLabelNetworkType: model.LabelValue(instance.InstanceNetworkType),
		}

		// check classic public ip
		if len(instance.PublicIpAddress.IpAddress) > 0 {
			labels[ecsLabelPublicIp] = model.LabelValue(instance.PublicIpAddress.IpAddress[0])
			addr := net.JoinHostPort(instance.PublicIpAddress.IpAddress[0], fmt.Sprintf("%d", d.port))
			labels[model.AddressLabel] = model.LabelValue(addr)
		}

		// check classic inner ip
		if len(instance.InnerIpAddress.IpAddress) > 0 {
			labels[ecsLabelInnerIp] = model.LabelValue(instance.InnerIpAddress.IpAddress[0])
			addr := net.JoinHostPort(instance.InnerIpAddress.IpAddress[0], fmt.Sprintf("%d", d.port))
			labels[model.AddressLabel] = model.LabelValue(addr)
		}

		// check vpc eip
		if instance.EipAddress.IpAddress != "" {
			labels[ecsLabelEip] = model.LabelValue(instance.EipAddress.IpAddress)
			addr := net.JoinHostPort(instance.EipAddress.IpAddress, fmt.Sprintf("%d", d.port))
			labels[model.AddressLabel] = model.LabelValue(addr)
		}

		// check vpc private ip
		if len(instance.VpcAttributes.PrivateIpAddress.IpAddress) > 0 {
			labels[ecsLabelPrivateIp] = model.LabelValue(instance.VpcAttributes.PrivateIpAddress.IpAddress[0])
			addr := net.JoinHostPort(instance.VpcAttributes.PrivateIpAddress.IpAddress[0], fmt.Sprintf("%d", d.port))
			labels[model.AddressLabel] = model.LabelValue(addr)
		}

		// tags
		for _, tag := range instance.Tags.Tag {
			labels[ecsLabelTag+model.LabelName(tag.TagKey)] = model.LabelValue(tag.TagValue)
		}

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}

func getEcsClient(config *SDConfig) (client *ecs_pop.Client, err error) {

	if config.RegionId == "" {
		return nil, errors.New("Aliyun ECS service discovery config need regionId.")
	}

	// NewClientWithRamRoleArnAndPolicy
	if config.Policy != "" && config.AccessKey != "" && config.AccessKeySecret != "" && config.RoleArn != "" && config.RoleSessionName != "" {
		client, clientErr := ecs_pop.NewClientWithRamRoleArnAndPolicy(config.RegionId, config.AccessKey, config.AccessKeySecret, config.RoleArn, config.RoleSessionName, config.Policy)
		return client, clientErr
	}

	// NewClientWithRamRoleArn
	if config.RoleSessionName != "" && config.AccessKey != "" && config.AccessKeySecret != "" && config.RoleArn != "" {
		client, clientErr := ecs_pop.NewClientWithRamRoleArn(config.RegionId, config.AccessKey, config.AccessKeySecret, config.RoleArn, config.RoleSessionName)
		return client, clientErr
	}
	
	// NewClientWithStsToken
	if config.StsToken != "" && config.AccessKey != "" && config.AccessKeySecret != "" {
		client, clientErr := ecs_pop.NewClientWithStsToken(config.RegionId, config.AccessKey, config.AccessKeySecret, config.StsToken)
		return client, clientErr
	}
	
	// NewClientWithAccessKey
	if config.AccessKey != "" && config.AccessKeySecret != "" {
		client, clientErr := ecs_pop.NewClientWithAccessKey(config.RegionId, config.AccessKey, config.AccessKeySecret)
		return client, clientErr
	}
	
	// NewClientWithEcsRamRole
	if config.RoleName != "" {
		client, clientErr := ecs_pop.NewClientWithEcsRamRole(config.RegionId, config.RoleName)
		return client, clientErr
	}
	
	// NewClientWithRsaKeyPair
	if config.PublicKeyId != "" && config.PrivateKey != "" && config.SessionExpiration != 0 {
		client, clientErr := ecs_pop.NewClientWithRsaKeyPair(config.RegionId, config.PublicKeyId, config.PrivateKey, config.SessionExpiration)
		return client, clientErr
	}
	
	return nil, errors.New("Aliyun ECS service discovery cant init client, need auth config.")
	
}
