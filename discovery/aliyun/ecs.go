// Copyright 2024 The Prometheus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aliyun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/denverdino/aliyungo/metadata"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	ecs_pop "github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
)

const (
	ecsLabel = model.MetaLabelPrefix + "ecs_"

	ecsLabelPublicIP  = ecsLabel + "public_ip"  // classic public ip
	ecsLabelInnerIP   = ecsLabel + "inner_ip"   // classic inner ip
	ecsLabelEip       = ecsLabel + "eip"        // vpc public eip
	ecsLabelPrivateIP = ecsLabel + "private_ip" // vpc private ip

	ecsLabelInstanceID  = ecsLabel + "instance_id"
	ecsLabelRegionID    = ecsLabel + "region_id"
	ecsLabelStatus      = ecsLabel + "status"
	ecsLabelZoneID      = ecsLabel + "zone_id"
	ecsLabelNetworkType = ecsLabel + "network_type"
	ecsLabelUserID      = ecsLabel + "user_id"
	ecsLabelTag         = ecsLabel + "tag_"

	MaxPageLimit = 50 // it's limited by ecs describeInstances API
)

var DefaultECSConfig = ECSConfig{
	Port:            8888,
	RefreshInterval: model.Duration(60 * time.Second),
	Limit:           100,
}

// ECSConfig is the configuration for Aliyun based service discovery.
type ECSConfig struct {
	Port            int            `yaml:"port"`
	UserID          string         `yaml:"user_id,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	RegionID        string         `yaml:"region_id,omitempty"`
	TagFilters      []*TagFilter   `yaml:"tag_filters"`

	// Alibaba ECS Auth Args
	// https://github.com/aliyun/alibaba-cloud-sdk-go/blob/master/docs/2-Client-EN.md
	AccessKey         string `yaml:"access_key,omitempty"`
	AccessKeySecret   string `yaml:"access_key_secret,omitempty"`
	StsToken          string `yaml:"sts_token,omitempty"`
	RoleArn           string `yaml:"role_arn,omitempty"`
	RoleSessionName   string `yaml:"role_session_name,omitempty"`
	Policy            string `yaml:"policy,omitempty"`
	RoleName          string `yaml:"role_name,omitempty"`
	PublicKeyID       string `yaml:"public_key_id,omitempty"`
	PrivateKey        string `yaml:"private_key,omitempty"`
	SessionExpiration int    `yaml:"session_expiration,omitempty"`

	// query ecs limit, default is 100.
	Limit int `yaml:"limit,omitempty"`
}

func init() {
	discovery.RegisterConfig(&ECSConfig{})
}

// Name returns the name of the ECS Config.
func (*ECSConfig) Name() string { return "ecs" }

// NewDiscoverer returns a Discoverer for the ECS Config.
func (c *ECSConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewECSDiscovery(c, opts.Logger, opts.Metrics)
}

// NewDiscovererMetrics implements discovery.Config.
func (*ECSConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return newDiscovererMetrics(reg, rmi)
}

func (c *ECSConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultECSConfig
	type plain ECSConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	for _, f := range c.TagFilters {
		if len(f.Values) == 0 {
			return errors.New("ECS SD configuration filter values cannot be empty")
		}
	}
	if len(c.RegionID) == 0 {
		return errors.New("ECS SD configuration need RegionId")
	}
	return nil
}

// Filter is the configuration tags for filtering ECS instances.
type TagFilter struct {
	Key    string   `yaml:"key"`
	Values []string `yaml:"values"`
}

type Discovery struct {
	*refresh.Discovery
	logger log.Logger
	ecsCfg *ECSConfig
	port   int
	limit  int

	tgCache *targetgroup.Group
	metrics *ecsMetrics
}

// NewECSDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewECSDiscovery(cfg *ECSConfig, logger log.Logger, metrics discovery.DiscovererMetrics) (discovery.Discoverer, error) {
	m, ok := metrics.(*ecsMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &Discovery{
		ecsCfg:  cfg,
		port:    cfg.Port,
		limit:   cfg.Limit,
		logger:  logger,
		tgCache: &targetgroup.Group{},
		metrics: m,
	}

	d.Discovery = refresh.NewDiscovery(refresh.Options{
		Logger:              logger,
		Mech:                "ecs",
		Interval:            time.Duration(cfg.RefreshInterval),
		RefreshF:            d.refresh,
		MetricsInstantiator: m.refreshMetrics,
	})
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	defer level.Debug(d.logger).Log("msg", "ECS discovery completed")

	d.metrics.queryCount.Inc()

	level.Info(d.logger).Log("msg", "New ECS Client with config and logger.")
	client, err := newECSClient(d.ecsCfg, d.logger)
	if err != nil {
		level.Debug(d.logger).Log("msg", "newECSClient", "err: ", err)
		return nil, err
	}

	level.Info(d.logger).Log("msg", "Query Instances with Aliyun OpenAPI.")
	instances, err := client.QueryInstances(d.ecsCfg.TagFilters, d.tgCache)
	if err != nil {
		level.Debug(d.logger).Log("msg", "QueryInstances", "err: ", err)
		d.metrics.queryFailuresCount.Inc()
		return nil, err
	}

	// build instances list.
	level.Info(d.logger).Log("msg", "Found Instances from remote during ECS discovery.", "count", len(instances))

	tg := &targetgroup.Group{
		Source: d.ecsCfg.RegionID,
	}

	noIPAddressInstanceCount := 0
	for _, instance := range instances {
		labels, err := addLabel(d.ecsCfg.UserID, d.port, instance)
		if err != nil {
			noIPAddressInstanceCount++
			level.Debug(d.logger).Log("msg", "Instance dont have AddressLabel.", "instance: ", fmt.Sprintf("%v", instance))
			continue
		}
		tg.Targets = append(tg.Targets, labels)
	}

	level.Info(d.logger).Log("msg", "Found Instances during ECS discovery.", "count", len(tg.Targets))
	if noIPAddressInstanceCount > 0 {
		level.Info(d.logger).Log("msg", "Found no AddressLabel instances during ECS discovery.", "count", noIPAddressInstanceCount)
	}

	// cache targetGroup
	d.tgCache = tg

	return []*targetgroup.Group{tg}, nil
}

func (cl *ecsClient) QueryInstances(tagFilters []*TagFilter, cache *targetgroup.Group) ([]ecs_pop.Instance, error) {
	if len(tagFilters) > 0 { // len(tagFilters) = 0 when tagFilters = nil
		// 1. tagFilter situation. query ListTagResources first, then query DescribeInstances
		instancesFromListTagResources, err := cl.queryFromListTagResources(tagFilters)
		if err != nil {
			level.Debug(cl.logger).Log("msg", "Query Instances from ListTagResources during ECS discovery.", "err", err)
			return nil, err
		}
		return instancesFromListTagResources, nil
	}

	// 2. no tagFilter situation. query DescribeInstances, then do cache double check.
	instancesFromDescribeInstances, err := cl.queryFromDescribeInstances()
	if err != nil {
		level.Debug(cl.logger).Log("msg", "Query Instances from DescribeInstances during ECS discovery.", "err", err)
		return nil, fmt.Errorf("query from DescribeInstances in QueryInstances, err: %w", err)
	}
	instancesFromCacheReCheck := cl.getCacheReCheckInstances(cache)
	level.Info(cl.logger).Log("msg", "Found Instances from cache re-check during ECS discovery.", "count", len(instancesFromCacheReCheck))
	instances := mergeHashInstances(instancesFromDescribeInstances, instancesFromCacheReCheck)
	return instances, nil
}

// listTagInstanceIDs get instance ids and filterred by tag.
func (cl *ecsClient) listTagInstanceIDs(token string, tagFilters []*TagFilter) ([]string, string, error) {
	listTagResourcesRequest := ecs_pop.CreateListTagResourcesRequest()
	listTagResourcesRequest.RegionId = cl.regionID
	listTagResourcesRequest.ResourceType = "instance"

	// FIRST token is empty, and continue
	if token != "FIRST" {
		if token != "" && token != "ICM=" {
			listTagResourcesRequest.NextToken = token
		} else {
			return []string{}, "", errors.New("token is empty, but not first request")
		}
	}

	filters := tagFiltersCast(tagFilters)
	listTagResourcesRequest.TagFilter = &filters
	response, err := cl.ListTagResources(listTagResourcesRequest)
	if err != nil {
		return []string{}, "", fmt.Errorf("response from ListTagResources, err: %w", err)
	}
	level.Debug(cl.logger).Log("msg", "get response from ListTagResources.", "response: ", response)

	tagResources := response.TagResources.TagResource
	if len(tagResources) == 0 { // len(tagResources) = 0 when tagResources = nil
		level.Debug(cl.logger).Log("msg", "ListTagResourcesTagFilter found no resources.", "response: ", response)
		return []string{}, "", nil
	}

	var resourceIDs []string
	for _, tagResource := range tagResources {
		resourceIDs = append(resourceIDs, tagResource.ResourceId)
	}
	level.Debug(cl.logger).Log("msg", "listTagResource and get ECS instanceIds. for ListTagResourcesTagFilter.", "instanceIds: ", resourceIDs)
	return resourceIDs, response.NextToken, nil
}

func tagFiltersCast(tagFilters []*TagFilter) []ecs_pop.ListTagResourcesTagFilter {
	var ret []ecs_pop.ListTagResourcesTagFilter
	for _, tagFilter := range tagFilters {
		if len(tagFilter.Values) == 0 {
			continue
		}
		tagFilter := ecs_pop.ListTagResourcesTagFilter{
			TagKey:    tagFilter.Key,
			TagValues: &tagFilter.Values,
		}
		ret = append(ret, tagFilter)
	}
	return ret
}

func (cl *ecsClient) queryFromDescribeInstances() ([]ecs_pop.Instance, error) {
	describeInstancesRequest := ecs_pop.CreateDescribeInstancesRequest()
	describeInstancesRequest.RegionId = cl.regionID
	describeInstancesRequest.PageSize = requests.NewInteger(MaxPageLimit)

	var instances []ecs_pop.Instance
	pageNumber := 1
	neededCount := MaxPageLimit // number of instances still needed
	if cl.limit >= 0 {
		neededCount = cl.limit
	}

	for neededCount > 0 {
		describeInstancesRequest.PageNumber = requests.NewInteger(pageNumber)
		response, err := cl.DescribeInstances(describeInstancesRequest)
		if err != nil {
			return nil, fmt.Errorf("could not get ecs describeInstances response, err: %w", err)
		}

		count := len(response.Instances.Instance)
		if count == 0 {
			break
		}
		// first page
		if pageNumber == 1 {
			neededCount = response.TotalCount
			if cl.limit > 0 {
				neededCount = min(neededCount, cl.limit)
			}
		}
		// if current page is last page, neededCount < count
		// else neededCount >= count
		pageLimit := min(count, neededCount) // number of instances in one page
		instances = append(instances, response.Instances.Instance[:pageLimit]...)

		neededCount -= count
		pageNumber++
	}
	return instances, nil
}

// getCacheReCheckInstances
// get cache targetGroup's instanceIDs, and query DescribeInstances again to double check.
// every 50 instance per page.
func (cl *ecsClient) getCacheReCheckInstances(cache *targetgroup.Group) []ecs_pop.Instance {
	pageCount := 0
	var instanceIDs []string
	var retInstances []ecs_pop.Instance
	for tgLabelSetIndex, tgLabelSet := range cache.Targets {
		pageCount++

		instanceID := tgLabelSet[ecsLabelInstanceID]
		instanceIDs = append(instanceIDs, string(instanceID))

		// full of one page, or last one of LabelSet Series.
		if pageCount >= MaxPageLimit || tgLabelSetIndex == (len(cache.Targets)-1) {
			// query instances
			instances, err := cl.describeInstances(instanceIDs)
			if err != nil {
				level.Error(cl.logger).Log("msg", "getCacheReCheckInstances describeInstancesResponse err.", "err: ", err)
				continue
			}

			retInstances = append(retInstances, instances...)
			// clean page
			pageCount = 0
			instanceIDs = []string{}
		}
	}
	return retInstances
}

// queryFromListTagResources
// token query.
func (cl *ecsClient) queryFromListTagResources(tagFilters []*TagFilter) ([]ecs_pop.Instance, error) {
	var currentInstances []ecs_pop.Instance
	var retInstances []ecs_pop.Instance
	var err error

	token := "FIRST"
	currentTotalCount := 0 // the number of instances that have been queried
	originalToken := "INIT"
	for {
		if token == "" || token == "ICM=" || token == originalToken {
			break
		}

		originalToken = token
		token, currentInstances, err = cl.listTagInstances(token, currentTotalCount, tagFilters)
		if err != nil {
			return nil, fmt.Errorf("list tag instances, err: %w", err)
		}

		if len(currentInstances) == 0 {
			break
		}
		currentTotalCount += len(currentInstances)
		retInstances = append(retInstances, currentInstances...)
	}
	return retInstances, nil
}

// describeInstances get instance
// page query, max size 50 every page.
func (cl *ecsClient) describeInstances(ids []string) ([]ecs_pop.Instance, error) {
	describeInstancesRequest := ecs_pop.CreateDescribeInstancesRequest()
	describeInstancesRequest.RegionId = cl.regionID
	describeInstancesRequest.PageNumber = requests.NewInteger(1)
	describeInstancesRequest.PageSize = requests.NewInteger(MaxPageLimit)

	if ids == nil {
		ids = []string{}
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return []ecs_pop.Instance{}, err
	}
	describeInstancesRequest.InstanceIds = string(idsJSON)

	response, err := cl.DescribeInstances(describeInstancesRequest)
	if err != nil {
		return []ecs_pop.Instance{}, fmt.Errorf("could not invoke DescribeInstances API, err: %w", err)
	}
	return response.Instances.Instance, nil
}

func (cl *ecsClient) listTagInstances(token string, currentTotalCount int, tagFilters []*TagFilter) (string, []ecs_pop.Instance, error) {
	ids, nextToken, err := cl.listTagInstanceIDs(token, tagFilters)
	if err != nil {
		return "", nil, fmt.Errorf("get ecs instance ids, err: %w", err)
	}
	instances, err := cl.describeInstances(ids)
	if err != nil {
		return "", nil, fmt.Errorf("get ecs instances(ids: %s) err: %w", ids, err)
	}

	pageLimit := len(instances)
	currentLimit := cl.limit - currentTotalCount // remaining instance count
	if 0 <= currentLimit && currentLimit < pageLimit {
		pageLimit = currentLimit
	}

	return nextToken, instances[:pageLimit], nil
}

type client interface {
	DescribeInstances(request *ecs_pop.DescribeInstancesRequest) (response *ecs_pop.DescribeInstancesResponse, err error)
	ListTagResources(request *ecs_pop.ListTagResourcesRequest) (response *ecs_pop.ListTagResourcesResponse, err error)
}

var _ client = &ecsClient{}

type ecsClient struct {
	regionID string
	limit    int
	client
	logger log.Logger
}

func newECSClient(config *ECSConfig, logger log.Logger) (*ecsClient, error) {
	cli, err := getClient(config, logger)
	if err != nil {
		return nil, err
	}
	return &ecsClient{
		regionID: config.RegionID,
		limit:    config.Limit,
		client:   cli,
		logger:   logger,
	}, nil
}

func getClient(config *ECSConfig, logger log.Logger) (*ecs_pop.Client, error) {
	level.Debug(logger).Log("msg", "Start to get Ecs Client.")

	if config.RegionID == "" {
		return nil, errors.New("aliyun ECS service discovery config need regionId")
	}

	// 1. Args

	// NewClientWithRamRoleArnAndPolicy
	if config.Policy != "" && config.AccessKey != "" && config.AccessKeySecret != "" && config.RoleArn != "" && config.RoleSessionName != "" {
		client, clientErr := ecs_pop.NewClientWithRamRoleArnAndPolicy(config.RegionID, config.AccessKey, config.AccessKeySecret, config.RoleArn, config.RoleSessionName, config.Policy)
		return client, clientErr
	}

	// NewClientWithRamRoleArn
	if config.RoleSessionName != "" && config.AccessKey != "" && config.AccessKeySecret != "" && config.RoleArn != "" {
		client, clientErr := ecs_pop.NewClientWithRamRoleArn(config.RegionID, config.AccessKey, config.AccessKeySecret, config.RoleArn, config.RoleSessionName)
		return client, clientErr
	}

	// NewClientWithStsToken
	if config.StsToken != "" && config.AccessKey != "" && config.AccessKeySecret != "" {
		client, clientErr := ecs_pop.NewClientWithStsToken(config.RegionID, config.AccessKey, config.AccessKeySecret, config.StsToken)
		return client, clientErr
	}

	// NewClientWithAccessKey
	if config.AccessKey != "" && config.AccessKeySecret != "" {
		client, clientErr := ecs_pop.NewClientWithAccessKey(config.RegionID, config.AccessKey, config.AccessKeySecret)
		return client, clientErr
	}

	// NewClientWithEcsRamRole
	if config.RoleName != "" {
		client, clientErr := ecs_pop.NewClientWithEcsRamRole(config.RegionID, config.RoleName)
		return client, clientErr
	}

	// NewClientWithRsaKeyPair
	if config.PublicKeyID != "" && config.PrivateKey != "" && config.SessionExpiration != 0 {
		client, clientErr := ecs_pop.NewClientWithRsaKeyPair(config.RegionID, config.PublicKeyID, config.PrivateKey, config.SessionExpiration)
		return client, clientErr
	}

	level.Debug(logger).Log("msg", "Start to get Ecs Client from ram.")

	// 2. ACS
	// get all RoleName for check

	metaData := metadata.NewMetaData(nil)
	var allRoleName metadata.ResultList
	allRoleNameErr := metaData.New().Resource("ram/security-credentials/").Do(&allRoleName)
	if allRoleNameErr != nil {
		level.Error(logger).Log("msg", "Get ECS Client from ram allRoleNameErr.", "err: ", allRoleNameErr)
		return nil, errors.New("aliyun ECS service discovery cant init client, need auth config")
	}

	roleName, roleNameErr := metaData.RoleName()
	level.Debug(logger).Log("msg", "Start to get Ecs Client from ram2.")

	if roleNameErr != nil {
		level.Error(logger).Log("msg", "Get ECS Client from ram roleNameErr.", "err: ", roleNameErr)
		return nil, errors.New("aliyun ECS service discovery cant init client, need auth config")
	}
	roleAuth, roleAuthErr := metaData.RamRoleToken(roleName)

	level.Debug(logger).Log("msg", "Start to get Ecs Client from ram3.")

	if roleAuthErr != nil {
		level.Error(logger).Log("msg", "Get ECS Client from ram roleAuthErr.", "err: ", roleAuthErr)
		return nil, errors.New("aliyun ECS service discovery cant init client, need auth config")
	}
	client := ecs_pop.Client{}
	clientConfig := client.InitClientConfig()
	clientConfig.Debug = true
	clientErr := client.InitWithStsToken(config.RegionID, roleAuth.AccessKeyId, roleAuth.AccessKeySecret, roleAuth.SecurityToken)

	level.Debug(logger).Log("msg", "Start to get Ecs Client from ram4.")

	if clientErr != nil {
		level.Error(logger).Log("msg", "Get ECS Client from ram clientErr.", "err: ", clientErr)
		return nil, errors.New("aliyun ECS service discovery cant init client, need auth config")
	}
	return &client, nil
}

// mergeHashInstances hash by instanceId and merge.
// O(n + m)
// The purpose is to remove duplicate elements.
func mergeHashInstances(instances1, instances2 []ecs_pop.Instance) []ecs_pop.Instance {
	instancesMap := make(map[string]ecs_pop.Instance)
	for _, each := range instances1 {
		instancesMap[each.InstanceId] = each
	}
	for _, each := range instances2 {
		instancesMap[each.InstanceId] = each
	}

	var instances []ecs_pop.Instance
	for _, eachInstance := range instancesMap {
		instances = append(instances, eachInstance)
	}
	return instances
}

// addLabel add label, return LabelSet and error (!=nil when isAddressLabelExist equal to false).
func addLabel(userID string, port int, instance ecs_pop.Instance) (model.LabelSet, error) {
	labels := model.LabelSet{
		ecsLabelInstanceID:  model.LabelValue(instance.InstanceId),
		ecsLabelRegionID:    model.LabelValue(instance.RegionId),
		ecsLabelStatus:      model.LabelValue(instance.Status),
		ecsLabelZoneID:      model.LabelValue(instance.ZoneId),
		ecsLabelNetworkType: model.LabelValue(instance.InstanceNetworkType),
	}

	if userID != "" {
		labels[ecsLabelUserID] = model.LabelValue(userID)
	}
	portStr := strconv.Itoa(port)

	// instance must have AddressLabel
	isAddressLabelExist := false

	// check classic public ip
	if len(instance.PublicIpAddress.IpAddress) > 0 {
		labels[ecsLabelPublicIP] = model.LabelValue(instance.PublicIpAddress.IpAddress[0])
		addr := net.JoinHostPort(instance.PublicIpAddress.IpAddress[0], portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	// check classic inner ip
	if len(instance.InnerIpAddress.IpAddress) > 0 {
		labels[ecsLabelInnerIP] = model.LabelValue(instance.InnerIpAddress.IpAddress[0])
		addr := net.JoinHostPort(instance.InnerIpAddress.IpAddress[0], portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	// check vpc eip
	if instance.EipAddress.IpAddress != "" {
		labels[ecsLabelEip] = model.LabelValue(instance.EipAddress.IpAddress)
		addr := net.JoinHostPort(instance.EipAddress.IpAddress, portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	// check vpc private ip
	if len(instance.VpcAttributes.PrivateIpAddress.IpAddress) > 0 {
		labels[ecsLabelPrivateIP] = model.LabelValue(instance.VpcAttributes.PrivateIpAddress.IpAddress[0])
		addr := net.JoinHostPort(instance.VpcAttributes.PrivateIpAddress.IpAddress[0], portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	if !isAddressLabelExist {
		return nil, fmt.Errorf("instance %s dont have AddressLabel", instance.InstanceId)
	}

	// tags
	for _, tag := range instance.Tags.Tag {
		labels[ecsLabelTag+model.LabelName(tag.TagKey)] = model.LabelValue(tag.TagValue)
	}
	return labels, nil
}
