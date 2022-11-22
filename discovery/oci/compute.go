// Copyright 2022 The Prometheus Authors
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
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"

	"github.com/prometheus/common/config"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	ociLabel                    = model.MetaLabelPrefix + "oci_compute_"
	ociLabelRegion              = ociLabel + "region" // Refer https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm.
	ociLabelAvailabilityDomain  = ociLabel + "availability_domain"
	ociLabelFaultDomain         = ociLabel + "fault_domain"
	ociLabelFullCompartmentName = ociLabel + "compartment"
	ociLabelCompartmentID       = ociLabel + "compartment_id"
	ociLabelInstanceID          = ociLabel + "instance_id"
	ociLabelInstanceName        = ociLabel + "instance_name"
	ociLabelInstanceState       = ociLabel + "instance_state"

	ociLabelDefinedTag  = ociLabel + "defined_tag_"
	ociLabelFreeformTag = ociLabel + "freeform_tag_"

	ociLabelOcpus                = ociLabel + "ocpus"
	ociLabelProcessorDescription = ociLabel + "processor_description"
	ociLabelMemoryInGbs          = ociLabel + "memory_bytes"
	ociLabelShape                = ociLabel + "shape"

	ociLabelPrivateIP              = ociLabel + "private_ip"
	ociLabelPublicIP               = ociLabel + "public_ip"
	ociLabelVcnID                  = ociLabel + "vcn_id"
	ociLabelVcn                    = ociLabel + "vcn"
	ociLabelSubnetID               = ociLabel + "subnet_id"
	ociLabelNetworkBandWidthInGbps = ociLabel + "networking_bandwidth_in_gbps"
)

// Default SD configuration.
var DefaultSDConfig = SDConfig{
	Port:            80,
	RefreshInterval: model.Duration(5 * 60 * time.Second),

	OciCreds: Authentication{Type: "InstancePrincipal"},
}

// SDConfig is the configuration for OCI based service discovery.
type SDConfig struct {
	Tenancy     string `yaml:"tenancy_ocid"`
	Region      string `yaml:"region"`
	Compartment string `yaml:"compartment_ocid,omitempty"`

	OciCreds Authentication `yaml:"authencation"`

	TagFilters []*FilterByTags `yaml:"tag_filters,omitempty"`

	Port            int            `yaml:"port"` // Port from where to scrape metrics on targets.
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

// Filter is the configuration for filtering OCI Compute instances, when discovering targets.
type FilterByTags struct {
	TagNameSpace string `yaml:"tag_namespace,omitempty"`
	TagKey       string `yaml:"tag_key,omitempty"`
	TagValue     string `yaml:"tag_value,omitempty"`
}

type Authentication struct {
	Type string `yaml:"type,omitempty"`

	OciConfigFilePath       string        `yaml:"oci_config_file_path,omitempty"`
	OciConfigFileProfile    string        `yaml:"oci_config_file_profile,omitempty"`
	OciConfigPvtKeyPassword config.Secret `yaml:"oci_config_file_pvt_key_password,omitempty"`
}

// Name returns the name of the 'OCI SD Discovery' Config for Prometheus.
func (*SDConfig) Name() string { return "oci_compute" }

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Tenancy == "" {
		return fmt.Errorf("oci_compute sd configuration: tenancy_ocid is required field")
	}
	if c.Region == "" {
		return fmt.Errorf("oci_compute sd configuration: region is required field")
	}
	if c.Compartment == "" {
		c.Compartment = c.Tenancy
	}
	if c.OciCreds.Type != "InstancePrincipal" && c.OciCreds.Type != "UserPrincipal" {
		return fmt.Errorf("oci_compute sd configuration: invalid type for authentication, valid types are InstancePrincipal or UserPrincipal")
	}

	if c.TagFilters != nil && len(c.TagFilters) > 0 {
		for _, filterByTags := range c.TagFilters {
			if filterByTags.TagKey == "" || filterByTags.TagValue == "" {
				return fmt.Errorf("oci_compute sd configuration: missing tag key or value, both tagKey and tagValue must be present for filtering")
			}
		}
	}

	return nil
}

// Discovery periodically performs OCI-SD requests. It implements the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	cfg       *SDConfig
	ociConfig common.ConfigurationProvider
	logger    log.Logger
	port      int

	computeClient *core.ComputeClient
	vnwClient     *core.VirtualNetworkClient
	iamClient     *identity.IdentityClient
}

func setupRetryPolicy(isInstancePrincipalUsed bool, logger log.Logger) {
	// Create global level customized retry policy.
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if r.Response != nil && r.Response.HTTPResponse().StatusCode == 200 {
			return false
		}
		// Refer https://docs.oracle.com/en-us/iaas/Content/API/References/apierrors.htm.
		if r.Error != nil && (common.IsErrorRetryableByDefault(r.Error) || common.IsCircuitBreakerError(r.Error)) {
			level.Debug(logger).Log("OCI sdk retriable Error", fmt.Sprintf("%#v", r))
			return true
		}
		if isInstancePrincipalUsed && r.Response.HTTPResponse().StatusCode == 401 {
			// For very rare possibility of race condition, when renewing token for InstancePrincipal.
			level.Debug(logger).Log("OCI sdk retriable Error during token renewal", fmt.Sprintf("%#v", r))
			return true
		}
		// No retries needed for unauthorized(non 401, 429) and 500.
		return false
	}

	globalLevelPolicy := common.NewRetryPolicyWithOptions(
		common.ReplaceWithValuesFromRetryPolicy(common.DefaultRetryPolicyWithoutEventualConsistency()),
		common.WithMaximumNumberAttempts(3),
		common.WithNextDuration(func(r common.OCIOperationResponse) time.Duration {
			return time.Millisecond * 300 * time.Duration(r.AttemptNumber)
		}),
		common.WithShouldRetryOperation(shouldRetry),
	)
	common.GlobalRetry = &globalLevelPolicy
}

// NewDiscovery returns a new OCI Discovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	ociConfig, err := getOciConfig(cfg)
	if err != nil {
		return nil, err
	}

	d := &Discovery{
		cfg:       cfg,
		port:      cfg.Port,
		logger:    logger,
		ociConfig: ociConfig,
	}

	setupRetryPolicy(cfg.OciCreds.Type == "InstancePrincipal", logger)

	computeClient, err := core.NewComputeClientWithConfigurationProvider(d.ociConfig)
	if err != nil {
		return nil, fmt.Errorf("error setting up OCI Compute Client: %w", err)
	}
	computeClient.SetRegion(cfg.Region)
	d.computeClient = &computeClient

	vnwClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(d.ociConfig)
	if err != nil {
		return nil, fmt.Errorf("error setting up OCI Virtual Network Client: %w", err)
	}
	vnwClient.SetRegion(cfg.Region)
	d.vnwClient = &vnwClient

	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(d.ociConfig)
	if err != nil {
		return nil, fmt.Errorf("error setting up OCI Identity Client: %w", err)
	}
	identityClient.SetRegion(cfg.Region)
	d.iamClient = &identityClient

	d.Discovery = refresh.NewDiscovery(
		logger,
		"oci_compute",
		time.Duration(cfg.RefreshInterval),
		d.refresh,
	)
	return d, nil
}

func getOciConfig(cfg *SDConfig) (common.ConfigurationProvider, error) {
	if cfg.OciCreds.Type == "UserPrincipal" {
		if cfg.OciCreds.OciConfigFilePath == "" {
			cfg.OciCreds.OciConfigFilePath = "~/.oci/config"
		}
		if cfg.OciCreds.OciConfigFileProfile == "" {
			cfg.OciCreds.OciConfigFileProfile = "DEFAULT"
		}

		ociConfig, err := common.ConfigurationProviderFromFileWithProfile(cfg.OciCreds.OciConfigFilePath, cfg.OciCreds.OciConfigFileProfile, string(cfg.OciCreds.OciConfigPvtKeyPassword))
		if err != nil {
			return nil, fmt.Errorf("Error setting up OCI Authentication Config Object, using UserPrincipal OCI IAM mechanism, using OciConfigFile: %w", err)
		}
		return ociConfig, nil
	} else if cfg.OciCreds.Type == "InstancePrincipal" {
		ociConfig, err := auth.InstancePrincipalConfigurationProviderForRegion(common.StringToRegion(cfg.Region))
		if err != nil {
			return nil, fmt.Errorf("Error setting up OCI Authentication Config Object, using InstancePrincipal OCI IAM mechanism: %w", err)
		}
		return ociConfig, nil
	}

	return nil, nil
}

// NewDiscoverer returns a Discoverer for the Compute Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger)
}

func (d *Discovery) refresh(context context.Context) ([]*targetgroup.Group, error) {
	computeClient := d.computeClient

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	cmptFullName, err := d.getCompartmentFullName(context)
	if err != nil {
		level.Error(d.logger).Log("msg", "Could not obtain compartment name", "ocid", d.cfg.Compartment)
		cmptFullName = ""
	}

	mapSubnetIDToDetails, err := d.getVcnInfoForCompartment(context)
	if err != nil {
		level.Error(d.logger).Log("msg", "Could not obtain vcn and subnet information for the the compartment", "ocid", d.cfg.Compartment)
		mapSubnetIDToDetails = make(map[string]SubnetInfo)
	}
	mapInstanceIDToVnicAttachment, err := d.getVnicIdsOfAllInstances(context)
	if err != nil {
		return nil, fmt.Errorf("could not obtain list of vnic-attachments: %w", err)
	}

	listInstancesReq := core.ListInstancesRequest{CompartmentId: &d.cfg.Compartment, Limit: common.Int(30), LifecycleState: core.InstanceLifecycleStateRunning}
	listInstancesFn := func(listInstancesReq core.ListInstancesRequest) (core.ListInstancesResponse, error) {
		return computeClient.ListInstances(context, listInstancesReq)
	}

	for listInstancesRes, err := listInstancesFn(listInstancesReq); ; listInstancesRes, err = listInstancesFn(listInstancesReq) {
		if err != nil {
			return nil, fmt.Errorf("could not obtain list of instances: %w", err)
		}

		for _, instance := range listInstancesRes.Items {
			if d.instanceMatchesFilterTags(instance) {

				networkLabelsForInstance, err := d.getInstanceVnicDetails(context, *instance.Id, mapSubnetIDToDetails, mapInstanceIDToVnicAttachment)
				if err != nil {
					level.Debug(d.logger).Log("msg", "Instance skipped from oci_compute service discovery as no network information found", "ocid", *instance.Id)
					continue
				}
				const gbToBytesMultiplier = 1073741824
				labels := model.LabelSet{
					ociLabelInstanceID:             model.LabelValue(*instance.Id),
					ociLabelInstanceName:           model.LabelValue(*instance.DisplayName),
					ociLabelInstanceState:          model.LabelValue(instance.LifecycleState),
					ociLabelCompartmentID:          model.LabelValue(*instance.CompartmentId),
					ociLabelAvailabilityDomain:     model.LabelValue(*instance.AvailabilityDomain),
					ociLabelFaultDomain:            model.LabelValue(*instance.FaultDomain),
					ociLabelRegion:                 model.LabelValue(*instance.Region),
					ociLabelOcpus:                  model.LabelValue(fmt.Sprintf("%.2f", *instance.ShapeConfig.Ocpus)),
					ociLabelMemoryInGbs:            model.LabelValue(fmt.Sprintf("%.2f", (*instance.ShapeConfig.MemoryInGBs)*gbToBytesMultiplier)),
					ociLabelShape:                  model.LabelValue(*instance.Shape),
					ociLabelProcessorDescription:   model.LabelValue(*instance.ShapeConfig.ProcessorDescription),
					ociLabelNetworkBandWidthInGbps: model.LabelValue(fmt.Sprintf("%.2f", *instance.ShapeConfig.NetworkingBandwidthInGbps)),
				}
				if cmptFullName != "" {
					labels[ociLabelFullCompartmentName] = model.LabelValue(cmptFullName)
				}
				for key, value := range instance.FreeformTags {
					labels[ociLabelFreeformTag+model.LabelName(strutil.SanitizeLabelName(key))] = model.LabelValue(value)
				}
				for ns, tags := range instance.DefinedTags {
					for key, value := range tags {
						labelName := model.LabelName(ociLabelDefinedTag + strutil.SanitizeLabelName(ns) + "_" + strutil.SanitizeLabelName(key))
						labels[labelName] = model.LabelValue(value.(string))
					}
				}
				labels = labels.Merge(*networkLabelsForInstance)
				tg.Targets = append(tg.Targets, labels)
			}
		}

		if listInstancesRes.OpcNextPage != nil {
			listInstancesReq.Page = listInstancesRes.OpcNextPage
		} else {
			break
		}
	}
	tgArr := []*targetgroup.Group{tg}
	level.Debug(d.logger).Log("msg", "oci_compute service discovery completed")
	return tgArr, nil
}

func (d *Discovery) getCompartmentFullName(ctx context.Context) (string, error) {
	tenancyOcid := d.cfg.Tenancy
	region := d.cfg.Region

	req := identity.GetTenancyRequest{TenancyId: common.String(tenancyOcid)}

	resp, err := d.iamClient.GetTenancy(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("Error when fetching OCI tenancy name for Tenancy %s: %w", tenancyOcid, err)
	}

	mapFromIDToName := make(map[string]string)
	mapFromIDToName[tenancyOcid] = "Tenancy(" + *resp.Name + ")" // Tenancy aka root compartment name.

	mapFromIDToParentCmptID := make(map[string]string)
	mapFromIDToParentCmptID[tenancyOcid] = "" // Since root cmpt does not have a parent.

	var page *string
	reg := common.StringToRegion(region)
	d.iamClient.SetRegion(string(reg))
	for {
		res, err := d.iamClient.ListCompartments(ctx,
			identity.ListCompartmentsRequest{
				CompartmentId:          &tenancyOcid,
				Page:                   page,
				AccessLevel:            identity.ListCompartmentsAccessLevelAny,
				CompartmentIdInSubtree: common.Bool(true),
			})
		if err != nil {
			return "", fmt.Errorf("Could not get list of compartments for tenancy %s: %w", tenancyOcid, err)
		}
		for _, compartment := range res.Items {
			if compartment.LifecycleState == identity.CompartmentLifecycleStateActive {
				mapFromIDToName[*(compartment.Id)] = *(compartment.Name)
				mapFromIDToParentCmptID[*(compartment.Id)] = *(compartment.CompartmentId)
			}
		}
		if res.OpcNextPage == nil {
			break
		}
		page = res.OpcNextPage
	}

	mapFromIDToFullCmptName := make(map[string]string)
	mapFromIDToFullCmptName[tenancyOcid] = mapFromIDToName[tenancyOcid]

	for len(mapFromIDToFullCmptName) < len(mapFromIDToName) {
		for cmptID, cmptParentCmptID := range mapFromIDToParentCmptID {
			_, isCmptNameResolvedFullyAlready := mapFromIDToFullCmptName[cmptID]
			if !isCmptNameResolvedFullyAlready {
				if cmptParentCmptID == tenancyOcid {
					mapFromIDToFullCmptName[cmptID] = mapFromIDToName[tenancyOcid] + "/" + mapFromIDToName[cmptID]
				} else {
					fullNameOfParentCmpt, isMyParentNameResolvedFully := mapFromIDToFullCmptName[cmptParentCmptID]
					if isMyParentNameResolvedFully {
						mapFromIDToFullCmptName[cmptID] = fullNameOfParentCmpt + "/" + mapFromIDToName[cmptID]
					}
				}
			}
		}
	}
	cmptFullName := mapFromIDToFullCmptName[d.cfg.Compartment]
	return cmptFullName, nil
}

func (d *Discovery) instanceMatchesFilterTags(instance core.Instance) bool {
	totalMatchCount := 0

	for _, tagFilter := range d.cfg.TagFilters {
		if tagFilter.TagNameSpace != "" {
			for ns, tags := range instance.DefinedTags {
				if tagFilter.TagNameSpace == ns {
					for key, value := range tags {
						if tagFilter.TagKey == key && tagFilter.TagValue == value {
							totalMatchCount++
							break
						}
					}
				}
			}
		} else {
			for key, value := range instance.FreeformTags {
				if tagFilter.TagKey == key && tagFilter.TagValue == value {
					totalMatchCount++
					break
				}
			}
		}
	}
	return totalMatchCount == len(d.cfg.TagFilters)
}

func (d *Discovery) getInstanceVnicDetails(ctx context.Context, instanceID string, mapSubnetIDToDetails map[string]SubnetInfo, mapInstanceIDToVnicAttachment map[string][]string) (*model.LabelSet, error) {
	labels := model.LabelSet{}
	instanceVnicList, isInstanceVnicListPresent := mapInstanceIDToVnicAttachment[instanceID]

	if isInstanceVnicListPresent {
		for _, vnicID := range instanceVnicList {
			vnicDetails, err := d.vnwClient.GetVnic(ctx,
				core.GetVnicRequest{VnicId: &vnicID},
			)
			if err != nil {
				level.Debug(d.logger).Log("msg", "Could not obtain vnic details ", "vnic-ocid", vnicID, "instance-ocid", instanceID)
				return nil, err
			}
			if *vnicDetails.IsPrimary {
				labels[ociLabelPrivateIP] = model.LabelValue(*vnicDetails.PrivateIp)
				addr := net.JoinHostPort(*vnicDetails.PrivateIp, fmt.Sprintf("%d", d.cfg.Port))
				if vnicDetails.PublicIp != nil && *vnicDetails.PublicIp != "" {
					labels[ociLabelPublicIP] = model.LabelValue(*vnicDetails.PublicIp)
				}
				labels[model.AddressLabel] = model.LabelValue(addr)
				labels[ociLabelSubnetID] = model.LabelValue(*vnicDetails.SubnetId)

				subnetVcnInfo, subnetinfoPresent := mapSubnetIDToDetails[*vnicDetails.SubnetId]
				if subnetinfoPresent {
					labels[ociLabelVcnID] = model.LabelValue(subnetVcnInfo.vcnOcid)
					labels[ociLabelVcn] = model.LabelValue(subnetVcnInfo.vcnName)
				} else {
					level.Debug(d.logger).Log("msg", "Could not obtain vcn info for ", "instance-ocid", instanceID, "vnic-ocid", vnicDetails.Id)
				}
				return &labels, nil
			}
		}
		return nil, fmt.Errorf("no primary vnic found for instance: %s", instanceID)
	}
	return nil, fmt.Errorf("list of vnicIds absent for instance: %s", instanceID)
}

func (d *Discovery) getVnicIdsOfAllInstances(ctx context.Context) (map[string][]string, error) {
	listVnicReq := core.ListVnicAttachmentsRequest{
		CompartmentId: &d.cfg.Compartment,
		Limit:         common.Int(1000),
	}
	listVnicsFn := func(listVnicReq core.ListVnicAttachmentsRequest) (core.ListVnicAttachmentsResponse, error) {
		return d.computeClient.ListVnicAttachments(
			ctx,
			listVnicReq,
		)
	}
	mapInstanceIDToVnicAttachment := make(map[string][]string)

	for listVnicsRes, err := listVnicsFn(listVnicReq); ; listVnicsRes, err = listVnicsFn(listVnicReq) {
		if err != nil {
			level.Debug(d.logger).Log("msg", "Could not obtain list of Vnics for compartmentId", "ocid", d.cfg.Compartment, "error", err.Error())
			return nil, err
		}

		for _, vnicAttachment := range listVnicsRes.Items {
			vnicAttachmentIDSlice, isEntryMade := mapInstanceIDToVnicAttachment[*vnicAttachment.InstanceId]
			if !isEntryMade {
				vnicAttachmentIDSlice = make([]string, 0)
			}
			vnicAttachmentIDSlice = append(vnicAttachmentIDSlice, *vnicAttachment.VnicId)
			mapInstanceIDToVnicAttachment[*vnicAttachment.InstanceId] = vnicAttachmentIDSlice
		}

		if listVnicsRes.OpcNextPage != nil {
			listVnicReq.Page = listVnicsRes.OpcNextPage
		} else {
			break
		}
	}

	return mapInstanceIDToVnicAttachment, nil
}

type SubnetInfo struct {
	vcnOcid string
	vcnName string
}

func (d *Discovery) getVcnInfoForCompartment(ctx context.Context) (map[string]SubnetInfo, error) {
	listVcnsReq := core.ListVcnsRequest{CompartmentId: &d.cfg.Compartment, Limit: common.Int(100)}
	listVcnsFn := func(listVcnsReq core.ListVcnsRequest) (core.ListVcnsResponse, error) {
		return d.vnwClient.ListVcns(ctx, listVcnsReq)
	}
	mapVcnIDToName := make(map[string]string)

	for listVcnsRes, err := listVcnsFn(listVcnsReq); ; listVcnsRes, err = listVcnsFn(listVcnsReq) {
		if err != nil {
			level.Debug(d.logger).Log("msg", "Could not obtain vcn list")
			return nil, err
		}

		for _, vcn := range listVcnsRes.Items {
			mapVcnIDToName[*vcn.Id] = *vcn.DisplayName
		}

		if listVcnsRes.OpcNextPage != nil {
			listVcnsReq.Page = listVcnsRes.OpcNextPage
		} else {
			break
		}
	}

	listSubnetsReq := core.ListSubnetsRequest{CompartmentId: &d.cfg.Compartment, Limit: common.Int(1000)}
	listSubnetsFn := func(listSubnetsReq core.ListSubnetsRequest) (core.ListSubnetsResponse, error) {
		return d.vnwClient.ListSubnets(ctx, listSubnetsReq)
	}
	mapSubNetDetails := make(map[string]SubnetInfo)

	for listSubnetsRes, err := listSubnetsFn(listSubnetsReq); ; listSubnetsRes, err = listSubnetsFn(listSubnetsReq) {
		if err != nil {
			level.Debug(d.logger).Log("msg", "Could not obtain subnet list")
			return nil, err
		}
		for _, sbnet := range listSubnetsRes.Items {
			mapSubNetDetails[*sbnet.Id] = SubnetInfo{*sbnet.VcnId, mapVcnIDToName[*sbnet.VcnId]}
		}

		if listSubnetsRes.OpcNextPage != nil {
			listSubnetsReq.Page = listSubnetsRes.OpcNextPage
		} else {
			break
		}
	}

	return mapSubNetDetails, nil
}
