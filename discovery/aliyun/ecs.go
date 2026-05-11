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
	"log/slog"
	"net"
	"strconv"
	"time"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	ecs "github.com/alibabacloud-go/ecs-20140526/v7/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/aliyun/credentials-go/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
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

	MaxPageLimit = 50 // MaxPageLimit is limited by ecs describeInstances API
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

// TagFilter is the configuration tags for filtering ECS instances.
type TagFilter struct {
	Key    string   `yaml:"key"`
	Values []string `yaml:"values"`
}

type Discovery struct {
	*refresh.Discovery
	logger *slog.Logger
	ecsCfg *ECSConfig
	port   int
	limit  int

	tgCache *targetgroup.Group
	metrics *ecsMetrics
}

// NewECSDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewECSDiscovery(cfg *ECSConfig, logger *slog.Logger, metrics discovery.DiscovererMetrics) (discovery.Discoverer, error) {
	m, ok := metrics.(*ecsMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	if logger == nil {
		logger = promslog.NewNopLogger()
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
	d.metrics.queryCount.Inc()

	d.logger.Debug("Creating ECS client")
	client, err := newECSClient(d.ecsCfg, d.logger)
	if err != nil {
		d.logger.Debug("Failed to create ECS client", "err", err)
		return nil, err
	}

	d.logger.Debug("Querying ECS instances")
	instances, err := client.QueryInstances(d.ecsCfg.TagFilters, d.tgCache)
	if err != nil {
		d.logger.Debug("Failed to query instances", "err", err)
		d.metrics.queryFailuresCount.Inc()
		return nil, err
	}

	d.logger.Debug("Fetched instances from API", "count", len(instances))

	tg := &targetgroup.Group{
		Source: d.ecsCfg.RegionID,
	}

	noIPAddressInstanceCount := 0
	for _, instance := range instances {
		labels, err := addLabel(d.ecsCfg.UserID, d.port, instance)
		if err != nil {
			noIPAddressInstanceCount++
			d.logger.Debug("Skipping instance without address", "instance_id", dara.StringValue(instance.InstanceId))
			continue
		}
		tg.Targets = append(tg.Targets, labels)
	}

	d.logger.Info("ECS discovery completed", "targets", len(tg.Targets))
	if noIPAddressInstanceCount > 0 {
		d.logger.Warn("Instances without IP address skipped", "count", noIPAddressInstanceCount)
	}

	// cache targetGroup
	d.tgCache = tg

	return []*targetgroup.Group{tg}, nil
}

func (cl *ecsClient) QueryInstances(tagFilters []*TagFilter, cache *targetgroup.Group) ([]*ecs.DescribeInstancesResponseBodyInstancesInstance, error) {
	if len(tagFilters) > 0 { // len(tagFilters) = 0 when tagFilters = nil
		// 1. tagFilter situation. query ListTagResources first, then query DescribeInstances
		instancesFromListTagResources, err := cl.queryFromListTagResources(tagFilters)
		if err != nil {
			cl.logger.Debug("Failed to query instances via ListTagResources", "err", err)
			return nil, err
		}
		return instancesFromListTagResources, nil
	}

	// 2. no tagFilter situation. query DescribeInstances, then do cache double check.
	instancesFromDescribeInstances, err := cl.queryFromDescribeInstances()
	if err != nil {
		cl.logger.Debug("Failed to query instances via DescribeInstances", "err", err)
		return nil, fmt.Errorf("query from DescribeInstances in QueryInstances, err: %w", err)
	}
	instancesFromCacheReCheck := cl.getCacheReCheckInstances(cache)
	cl.logger.Debug("Re-checked cached instances", "count", len(instancesFromCacheReCheck))
	instances := mergeHashInstances(instancesFromDescribeInstances, instancesFromCacheReCheck)
	return instances, nil
}

// listTagInstanceIDs get instance ids and filterred by tag.
func (cl *ecsClient) listTagInstanceIDs(token string, tagFilters []*TagFilter) ([]string, string, error) {
	listTagResourcesRequest := &ecs.ListTagResourcesRequest{
		RegionId:     dara.String(cl.regionID),
		ResourceType: dara.String("instance"),
	}

	// FIRST token is empty, and continue
	if token != "FIRST" {
		if token != "" && token != "ICM=" {
			listTagResourcesRequest.NextToken = dara.String(token)
		} else {
			return []string{}, "", errors.New("token is empty, but not first request")
		}
	}

	filters := tagFiltersCast(tagFilters)
	listTagResourcesRequest.TagFilter = filters
	runtime := &dara.RuntimeOptions{}
	response, err := cl.ListTagResourcesWithOptions(listTagResourcesRequest, runtime)
	if err != nil {
		return []string{}, "", fmt.Errorf("response from ListTagResources, err: %w", err)
	}

	nextToken := dara.StringValue(response.Body.NextToken)
	cl.logger.Debug("Received ListTagResources response", "next_token", nextToken)

	var tagResources []*ecs.ListTagResourcesResponseBodyTagResourcesTagResource
	if response.Body != nil && response.Body.TagResources != nil {
		tagResources = response.Body.TagResources.TagResource
	}
	if len(tagResources) == 0 { // len(tagResources) = 0 when tagResources = nil
		cl.logger.Debug("No resources found for tag filters")
		return []string{}, "", nil
	}

	var resourceIDs []string
	for _, tagResource := range tagResources {
		resourceIDs = append(resourceIDs, dara.StringValue(tagResource.ResourceId))
	}
	cl.logger.Debug("Listed tag resources", "count", len(resourceIDs))
	return resourceIDs, nextToken, nil
}

func tagFiltersCast(tagFilters []*TagFilter) []*ecs.ListTagResourcesRequestTagFilter {
	var ret []*ecs.ListTagResourcesRequestTagFilter
	for _, tagFilter := range tagFilters {
		if len(tagFilter.Values) == 0 {
			continue
		}
		tagValues := make([]*string, 0, len(tagFilter.Values))
		for _, v := range tagFilter.Values {
			tagValues = append(tagValues, dara.String(v))
		}
		f := &ecs.ListTagResourcesRequestTagFilter{
			TagKey:    dara.String(tagFilter.Key),
			TagValues: tagValues,
		}
		ret = append(ret, f)
	}
	return ret
}

func (cl *ecsClient) queryFromDescribeInstances() ([]*ecs.DescribeInstancesResponseBodyInstancesInstance, error) {
	var instances []*ecs.DescribeInstancesResponseBodyInstancesInstance
	pageNumber := 1
	neededCount := MaxPageLimit // number of instances still needed
	if cl.limit >= 0 {
		neededCount = cl.limit
	}

	for neededCount > 0 {
		describeInstancesRequest := &ecs.DescribeInstancesRequest{
			RegionId:   dara.String(cl.regionID),
			PageSize:   dara.Int32(int32(MaxPageLimit)),
			PageNumber: dara.Int32(int32(pageNumber)),
		}
		runtime := &dara.RuntimeOptions{}
		response, err := cl.DescribeInstancesWithOptions(describeInstancesRequest, runtime)
		if err != nil {
			return nil, fmt.Errorf("could not get ecs describeInstances response, err: %w", err)
		}

		var instanceList []*ecs.DescribeInstancesResponseBodyInstancesInstance
		if response.Body != nil && response.Body.Instances != nil {
			instanceList = response.Body.Instances.Instance
		}

		count := len(instanceList)
		if count == 0 {
			break
		}
		// first page
		if pageNumber == 1 {
			neededCount = int(dara.Int32Value(response.Body.TotalCount))
			if cl.limit > 0 {
				neededCount = min(neededCount, cl.limit)
			}
		}
		// if current page is last page, neededCount < count
		// else neededCount >= count
		pageLimit := min(count, neededCount) // number of instances in one page
		instances = append(instances, instanceList[:pageLimit]...)

		neededCount -= count
		pageNumber++
	}
	return instances, nil
}

// getCacheReCheckInstances
// get cache targetGroup's instanceIDs, and query DescribeInstances again to double check.
// every 50 instance per page.
func (cl *ecsClient) getCacheReCheckInstances(cache *targetgroup.Group) []*ecs.DescribeInstancesResponseBodyInstancesInstance {
	pageCount := 0
	var instanceIDs []string
	var retInstances []*ecs.DescribeInstancesResponseBodyInstancesInstance
	for tgLabelSetIndex, tgLabelSet := range cache.Targets {
		pageCount++

		instanceID := tgLabelSet[ecsLabelInstanceID]
		instanceIDs = append(instanceIDs, string(instanceID))

		// full of one page, or last one of LabelSet Series.
		if pageCount >= MaxPageLimit || tgLabelSetIndex == (len(cache.Targets)-1) {
			// query instances
			instances, err := cl.describeInstances(instanceIDs)
			if err != nil {
				cl.logger.Warn("Failed to describe instances for cache re-check", "err", err)
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
func (cl *ecsClient) queryFromListTagResources(tagFilters []*TagFilter) ([]*ecs.DescribeInstancesResponseBodyInstancesInstance, error) {
	var currentInstances []*ecs.DescribeInstancesResponseBodyInstancesInstance
	var retInstances []*ecs.DescribeInstancesResponseBodyInstancesInstance
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
func (cl *ecsClient) describeInstances(ids []string) ([]*ecs.DescribeInstancesResponseBodyInstancesInstance, error) {
	if ids == nil {
		ids = []string{}
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}

	describeInstancesRequest := &ecs.DescribeInstancesRequest{
		RegionId:    dara.String(cl.regionID),
		PageNumber:  dara.Int32(1),
		PageSize:    dara.Int32(int32(MaxPageLimit)),
		InstanceIds: dara.String(string(idsJSON)),
	}

	runtime := &dara.RuntimeOptions{}
	response, err := cl.DescribeInstancesWithOptions(describeInstancesRequest, runtime)
	if err != nil {
		return nil, fmt.Errorf("could not invoke DescribeInstances API, err: %w", err)
	}

	if response.Body != nil && response.Body.Instances != nil {
		return response.Body.Instances.Instance, nil
	}
	return nil, nil
}

func (cl *ecsClient) listTagInstances(token string, currentTotalCount int, tagFilters []*TagFilter) (string, []*ecs.DescribeInstancesResponseBodyInstancesInstance, error) {
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
	DescribeInstancesWithOptions(request *ecs.DescribeInstancesRequest, runtime *dara.RuntimeOptions) (*ecs.DescribeInstancesResponse, error)
	ListTagResourcesWithOptions(request *ecs.ListTagResourcesRequest, runtime *dara.RuntimeOptions) (*ecs.ListTagResourcesResponse, error)
}

var _ client = &ecs.Client{}

type ecsClient struct {
	regionID string
	limit    int
	client
	logger *slog.Logger
}

func newECSClient(config *ECSConfig, logger *slog.Logger) (*ecsClient, error) {
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

func getClient(config *ECSConfig, logger *slog.Logger) (*ecs.Client, error) {
	logger.Debug("Resolving ECS client credentials", "region", config.RegionID)

	if config.RegionID == "" {
		return nil, errors.New("aliyun ECS service discovery config need regionId")
	}

	var credConfig *credentials.Config

	// Determine credential type based on config.
	if config.Policy != "" && config.AccessKey != "" && config.AccessKeySecret != "" && config.RoleArn != "" && config.RoleSessionName != "" {
		credConfig = &credentials.Config{
			Type:            dara.String("ram_role_arn"),
			AccessKeyId:     dara.String(config.AccessKey),
			AccessKeySecret: dara.String(config.AccessKeySecret),
			RoleArn:         dara.String(config.RoleArn),
			RoleSessionName: dara.String(config.RoleSessionName),
			Policy:          dara.String(config.Policy),
		}
	} else if config.RoleSessionName != "" && config.AccessKey != "" && config.AccessKeySecret != "" && config.RoleArn != "" {
		credConfig = &credentials.Config{
			Type:            dara.String("ram_role_arn"),
			AccessKeyId:     dara.String(config.AccessKey),
			AccessKeySecret: dara.String(config.AccessKeySecret),
			RoleArn:         dara.String(config.RoleArn),
			RoleSessionName: dara.String(config.RoleSessionName),
		}
	} else if config.StsToken != "" && config.AccessKey != "" && config.AccessKeySecret != "" {
		credConfig = &credentials.Config{
			Type:            dara.String("sts"),
			AccessKeyId:     dara.String(config.AccessKey),
			AccessKeySecret: dara.String(config.AccessKeySecret),
			SecurityToken:   dara.String(config.StsToken),
		}
	} else if config.AccessKey != "" && config.AccessKeySecret != "" {
		credConfig = &credentials.Config{
			Type:            dara.String("access_key"),
			AccessKeyId:     dara.String(config.AccessKey),
			AccessKeySecret: dara.String(config.AccessKeySecret),
		}
	} else if config.RoleName != "" {
		credConfig = &credentials.Config{
			Type:     dara.String("ecs_ram_role"),
			RoleName: dara.String(config.RoleName),
		}
	} else {
		// Default: use ECS RAM role from metadata.
		credConfig = &credentials.Config{
			Type: dara.String("ecs_ram_role"),
		}
	}

	cred, err := credentials.NewCredential(credConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create credentials: %w", err)
	}

	apiConfig := &openapiutil.Config{
		Credential: cred,
		RegionId:   dara.String(config.RegionID),
	}

	ecsClient, err := ecs.NewClient(apiConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECS client: %w", err)
	}
	return ecsClient, nil
}

// mergeHashInstances hash by instanceId and merge.
// O(n + m)
// The purpose is to remove duplicate elements.
func mergeHashInstances(instances1, instances2 []*ecs.DescribeInstancesResponseBodyInstancesInstance) []*ecs.DescribeInstancesResponseBodyInstancesInstance {
	instancesMap := make(map[string]*ecs.DescribeInstancesResponseBodyInstancesInstance)
	for _, each := range instances1 {
		instancesMap[dara.StringValue(each.InstanceId)] = each
	}
	for _, each := range instances2 {
		instancesMap[dara.StringValue(each.InstanceId)] = each
	}

	var instances []*ecs.DescribeInstancesResponseBodyInstancesInstance
	for _, eachInstance := range instancesMap {
		instances = append(instances, eachInstance)
	}
	return instances
}

// addLabel add label, return LabelSet and error (!=nil when isAddressLabelExist equal to false).
func addLabel(userID string, port int, instance *ecs.DescribeInstancesResponseBodyInstancesInstance) (model.LabelSet, error) {
	labels := model.LabelSet{
		ecsLabelInstanceID:  model.LabelValue(dara.StringValue(instance.InstanceId)),
		ecsLabelRegionID:    model.LabelValue(dara.StringValue(instance.RegionId)),
		ecsLabelStatus:      model.LabelValue(dara.StringValue(instance.Status)),
		ecsLabelZoneID:      model.LabelValue(dara.StringValue(instance.ZoneId)),
		ecsLabelNetworkType: model.LabelValue(dara.StringValue(instance.InstanceNetworkType)),
	}

	if userID != "" {
		labels[ecsLabelUserID] = model.LabelValue(userID)
	}
	portStr := strconv.Itoa(port)

	// instance must have AddressLabel
	isAddressLabelExist := false

	// check classic public ip
	if instance.PublicIpAddress != nil && len(instance.PublicIpAddress.IpAddress) > 0 {
		ip := dara.StringValue(instance.PublicIpAddress.IpAddress[0])
		labels[ecsLabelPublicIP] = model.LabelValue(ip)
		addr := net.JoinHostPort(ip, portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	// check classic inner ip
	if instance.InnerIpAddress != nil && len(instance.InnerIpAddress.IpAddress) > 0 {
		ip := dara.StringValue(instance.InnerIpAddress.IpAddress[0])
		labels[ecsLabelInnerIP] = model.LabelValue(ip)
		addr := net.JoinHostPort(ip, portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	// check vpc eip
	if instance.EipAddress != nil && dara.StringValue(instance.EipAddress.IpAddress) != "" {
		ip := dara.StringValue(instance.EipAddress.IpAddress)
		labels[ecsLabelEip] = model.LabelValue(ip)
		addr := net.JoinHostPort(ip, portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	// check vpc private ip
	if instance.VpcAttributes != nil && instance.VpcAttributes.PrivateIpAddress != nil && len(instance.VpcAttributes.PrivateIpAddress.IpAddress) > 0 {
		ip := dara.StringValue(instance.VpcAttributes.PrivateIpAddress.IpAddress[0])
		labels[ecsLabelPrivateIP] = model.LabelValue(ip)
		addr := net.JoinHostPort(ip, portStr)
		labels[model.AddressLabel] = model.LabelValue(addr)
		isAddressLabelExist = true
	}

	if !isAddressLabelExist {
		return nil, fmt.Errorf("instance %s dont have AddressLabel", dara.StringValue(instance.InstanceId))
	}

	// tags
	if instance.Tags != nil {
		for _, tag := range instance.Tags.Tag {
			labels[ecsLabelTag+model.LabelName(dara.StringValue(tag.TagKey))] = model.LabelValue(dara.StringValue(tag.TagValue))
		}
	}
	return labels, nil
}
