package ecs

import (
	"context"
	"fmt"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/denverdino/aliyungo/metadata"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"net"
	"os"
	"strconv"
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
	ecsLabelUserId      = ecsLabel + "user_id"
	ecsLabelTag         = ecsLabel + "tag_"

	MAX_PAGE_LIMIT = 100 // it's limited by ecs describeInstances API
)

// SDConfig is the configuration for Azure based service discovery.
type SDConfig struct {
	Port            int            `yaml:"port"`
	UserId          string         `yaml:"user_id,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	RegionId        string         `yaml:"region_id,omitempty"` // env set PROMETHEUS_DS_ECS_REGION_ID
	TagFilters         []*TagFilter      `yaml:"tag_filters"`

	// Alibaba ECS Auth Args
	// https://github.com/aliyun/alibaba-cloud-sdk-go/blob/master/docs/2-Client-EN.md
	AccessKey         string `yaml:"access_key,omitempty"`         // env set PROMETHEUS_DS_ECS_AK
	AccessKeySecret   string `yaml:"access_key_secret,omitempty"`  // env set PROMETHEUS_DS_ECS_SK
	StsToken          string `yaml:"sts_token,omitempty"`          // env set PROMETHEUS_DS_ECS_STS_TOKEN
	RoleArn           string `yaml:"role_arn,omitempty"`           // env set PROMETHEUS_DS_ECS_ROLE_ARN
	RoleSessionName   string `yaml:"role_session_name,omitempty"`  // env set PROMETHEUS_DS_ECS_ROLE_SESSION_NAME
	Policy            string `yaml:"policy,omitempty"`             // env set PROMETHEUS_DS_ECS_POLICY
	RoleName          string `yaml:"role_name,omitempty"`          // env set PROMETHEUS_DS_ECS_ROLE_NAME
	PublicKeyId       string `yaml:"public_key_id,omitempty"`      // env set PROMETHEUS_DS_ECS_PUBLIC_KEY_ID
	PrivateKey        string `yaml:"private_key,omitempty"`        // env set PROMETHEUS_DS_ECS_PRIVATE_KEY
	SessionExpiration int    `yaml:"session_expiration,omitempty"` // env set PROMETHEUS_DS_ECS_SESSION_EXPIRATION

	// query ecs limit, default is 100.
	Limit int `yaml:"limit,omitempty"`
}

// Filter is the configuration tags for filtering ECS instances.
type TagFilter struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

type Discovery struct {
	*refresh.Discovery
	logger log.Logger
	ecsCfg *SDConfig
	port   int
	limit  int
}

// NewDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &Discovery{
		ecsCfg: cfg,
		port:   cfg.Port,
		limit:  cfg.Limit,
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

	// tag filters
	var tagsFilters []ecs_pop.DescribeInstancesTag
	for _, tagFilter := range d.ecsCfg.TagFilters {
		tag := ecs_pop.DescribeInstancesTag{Key: tagFilter.Key, Value: tagFilter.Value}
		tagsFilters = append(tagsFilters, tag)
	}
	describeInstancesRequest.Tag = &tagsFilters

	// 分页查询
	var pageLimit = MAX_PAGE_LIMIT
	var currentLimit = d.limit
	var currentTotalCount = 0
	var totalCount = 0
	if d.limit <= 0 || d.limit > MAX_PAGE_LIMIT {
		pageLimit = MAX_PAGE_LIMIT
	} else {
		pageLimit = d.limit
	}
	describeInstancesRequest.PageNumber = requests.NewInteger(1)
	describeInstancesRequest.PageSize = requests.NewInteger(pageLimit)

	client, clientErr := getEcsClient(d.ecsCfg)
	if clientErr != nil {
		return nil, errors.Wrap(clientErr, "could not create alibaba ecs client.")
	}

	describeInstancesResponse, responseErr := client.DescribeInstances(describeInstancesRequest)
	if responseErr != nil {
		return nil, errors.Wrap(responseErr, "could not get ecs describeInstances response.")
	}

	// first query to get TotalCount
	instances := describeInstancesResponse.Instances.Instance
	currentTotalCount = len(describeInstancesResponse.Instances.Instance)
	totalCount = describeInstancesResponse.TotalCount
	if d.limit <= 0 {
		currentLimit = totalCount
	}

	// multi page query
	if currentTotalCount < currentLimit {

		for pageIndex := 2; currentTotalCount < currentLimit; pageIndex++ {
			if (currentLimit - currentTotalCount) < MAX_PAGE_LIMIT {
				pageLimit = currentLimit - currentTotalCount
			}
			describeInstancesRequest.PageNumber = requests.NewInteger(pageIndex)
			describeInstancesRequest.PageSize = requests.NewInteger(pageLimit)
			describeInstancesResponse, responseErr := client.DescribeInstances(describeInstancesRequest)
			if responseErr != nil {
				return nil, errors.Wrap(responseErr, "could not get ecs describeInstances response.")
			}
			for _, instance := range describeInstancesResponse.Instances.Instance {
				instances = append(instances, instance)
			}

			if describeInstancesResponse.PageSize == 0 {
				break
			}
			currentTotalCount += len(describeInstancesResponse.Instances.Instance)
		}

	}

	level.Debug(d.logger).Log("msg", "Found Instances during ECS discovery.", "count", len(instances))

	tg := &targetgroup.Group{
		Source: getConfigRegionId(d.ecsCfg.RegionId),
	}

	for _, instance := range instances {

		labels := model.LabelSet{
			ecsLabelInstanceId:  model.LabelValue(instance.InstanceId),
			ecsLabelRegionId:    model.LabelValue(instance.RegionId),
			ecsLabelStatus:      model.LabelValue(instance.Status),
			ecsLabelZoneId:      model.LabelValue(instance.ZoneId),
			ecsLabelNetworkType: model.LabelValue(instance.InstanceNetworkType),
		}

		if d.ecsCfg.UserId != "" {
			labels[ecsLabelUserId] = model.LabelValue(d.ecsCfg.UserId)
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

	if getConfigRegionId(config.RegionId) == "" {
		return nil, errors.New("Aliyun ECS service discovery config need regionId.")
	}

	// 1. Args

	// NewClientWithRamRoleArnAndPolicy
	if getConfigArgPolicy(config.Policy) != "" && getConfigArgAk(config.AccessKey) != "" && getConfigArgSk(config.AccessKeySecret) != "" && getConfigArgRoleArn(config.RoleArn) != "" && getConfigArgRoleSessionName(config.RoleSessionName) != "" {
		client, clientErr := ecs_pop.NewClientWithRamRoleArnAndPolicy(getConfigRegionId(config.RegionId), getConfigArgAk(config.AccessKey), getConfigArgSk(config.AccessKeySecret), getConfigArgRoleArn(config.RoleArn), getConfigArgRoleSessionName(config.RoleSessionName), getConfigArgPolicy(config.Policy))
		return client, clientErr
	}

	// NewClientWithRamRoleArn
	if getConfigArgRoleSessionName(config.RoleSessionName) != "" && getConfigArgAk(config.AccessKey) != "" && getConfigArgSk(config.AccessKeySecret) != "" && getConfigArgRoleArn(config.RoleArn) != "" {
		client, clientErr := ecs_pop.NewClientWithRamRoleArn(getConfigRegionId(config.RegionId), getConfigArgAk(config.AccessKey), getConfigArgSk(config.AccessKeySecret), getConfigArgRoleArn(config.RoleArn), getConfigArgRoleSessionName(config.RoleSessionName))
		return client, clientErr
	}

	// NewClientWithStsToken
	if getConfigArgStsToken(config.StsToken) != "" && getConfigArgAk(config.AccessKey) != "" && getConfigArgSk(config.AccessKeySecret) != "" {
		client, clientErr := ecs_pop.NewClientWithStsToken(getConfigRegionId(config.RegionId), getConfigArgAk(config.AccessKey), getConfigArgSk(config.AccessKeySecret), getConfigArgStsToken(config.StsToken))
		return client, clientErr
	}

	// NewClientWithAccessKey
	if getConfigArgAk(config.AccessKey) != "" && getConfigArgSk(config.AccessKeySecret) != "" {
		client, clientErr := ecs_pop.NewClientWithAccessKey(getConfigRegionId(config.RegionId), getConfigArgAk(config.AccessKey), getConfigArgSk(config.AccessKeySecret))
		return client, clientErr
	}

	// NewClientWithEcsRamRole
	if config.RoleName != "" {
		client, clientErr := ecs_pop.NewClientWithEcsRamRole(getConfigRegionId(config.RegionId), getConfigArgRoleName(config.RoleName))
		return client, clientErr
	}

	// NewClientWithRsaKeyPair
	if config.PublicKeyId != "" && config.PrivateKey != "" && config.SessionExpiration != 0 {
		client, clientErr := ecs_pop.NewClientWithRsaKeyPair(getConfigRegionId(config.RegionId), getConfigArgPublicKeyId(config.PublicKeyId), getConfigArgPrivateKey(config.PrivateKey), getConfigArgSessionExpiration(config.SessionExpiration))
		return client, clientErr
	}

	// 2. ACS
	//get all RoleName for check

	metaData := metadata.NewMetaData(nil)
	var allRoleName metadata.ResultList
	allRoleNameErr := metaData.New().Resource("ram/security-credentials/").Do(&allRoleName)
	if allRoleNameErr == nil {
		roleName, roleNameErr := metaData.RoleName()
		if roleNameErr == nil {
			roleAuth, roleAuthErr := metaData.RamRoleToken(roleName)
			if roleAuthErr == nil {
				client := ecs_pop.Client{}
				clientConfig := client.InitClientConfig()
				clientConfig.Debug = true
				clientErr := client.InitWithStsToken(getConfigRegionId(config.RegionId), roleAuth.AccessKeyId, roleAuth.AccessKeySecret, roleAuth.SecurityToken)
				if clientErr == nil {
					return &client, nil
				}
			}
		}
	}

	return nil, errors.New("Aliyun ECS service discovery cant init client, need auth config.")

}

func getConfigArgAk(ak string) string {
	akEnv := os.Getenv("PROMETHEUS_DS_ECS_AK")
	if akEnv != "" {
		return akEnv
	}
	return ak
}

func getConfigArgSk(sk string) string {
	skEnv := os.Getenv("PROMETHEUS_DS_ECS_SK")
	if skEnv != "" {
		return skEnv
	}
	return sk
}

func getConfigArgStsToken(stsToken string) string {
	stsEnv := os.Getenv("PROMETHEUS_DS_ECS_STS_TOKEN")
	if stsEnv != "" {
		return stsEnv
	}
	return stsToken
}

func getConfigArgRoleArn(roleArn string) string {
	roleArnEnv := os.Getenv("PROMETHEUS_DS_ECS_ROLE_ARN")
	if roleArnEnv != "" {
		return roleArnEnv
	}
	return roleArn
}

func getConfigArgRoleSessionName(roleSessionName string) string {
	roleSessionNameEnv := os.Getenv("PROMETHEUS_DS_ECS_ROLE_SESSION_NAME")
	if roleSessionNameEnv != "" {
		return roleSessionNameEnv
	}
	return roleSessionName
}

func getConfigArgPolicy(policy string) string {
	policyEnv := os.Getenv("PROMETHEUS_DS_ECS_POLICY")
	if policyEnv != "" {
		return policyEnv
	}
	return policy
}

func getConfigArgRoleName(roleName string) string {
	roleNameEnv := os.Getenv("PROMETHEUS_DS_ECS_ROLE_NAME")
	if roleNameEnv != "" {
		return roleNameEnv
	}
	return roleName
}

func getConfigArgPublicKeyId(publicKeyId string) string {
	publicKeyIdEnv := os.Getenv("PROMETHEUS_DS_ECS_PUBLIC_KEY_ID")
	if publicKeyIdEnv != "" {
		return publicKeyIdEnv
	}
	return publicKeyId
}

func getConfigArgPrivateKey(privateKey string) string {
	privateKeyEnv := os.Getenv("PROMETHEUS_DS_ECS_PRIVATE_KEY")
	if privateKeyEnv != "" {
		return privateKeyEnv
	}
	return privateKey
}

func getConfigArgSessionExpiration(sessionExpiration int) int {
	sessionExpirationEnv := os.Getenv("PROMETHEUS_DS_ECS_SESSION_EXPIRATION")
	if sessionExpirationEnv != "" {
		sessionExpirationEnvInt, err := strconv.Atoi(sessionExpirationEnv)
		if err != nil {
			return sessionExpirationEnvInt
		}
	}
	return sessionExpiration
}

func getConfigRegionId(regionId string) string {
	regionIdEnv := os.Getenv("PROMETHEUS_DS_ECS_REGION_ID")
	if regionIdEnv != "" {
		return regionIdEnv
	}
	return regionId
}
