// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

// Core Services API
//
// APIs for Networking Service, Compute Service, and Block Volume Service.
//

package core

import (
	"encoding/json"
	"github.com/oracle/oci-go-sdk/common"
)

// VolumeGroup Specifies a volume group which is a collection of volumes. For more information, see Volume Groups (https://docs.us-phoenix-1.oraclecloud.com/Content/Block/Concepts/volumegroups.htm).
type VolumeGroup struct {

	// The availability domain of the volume group.
	AvailabilityDomain *string `mandatory:"true" json:"availabilityDomain"`

	// The OCID of the compartment that contains the volume group.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// A user-friendly name for the volume group. Does not have to be unique, and it's changeable. Avoid entering confidential information.
	DisplayName *string `mandatory:"true" json:"displayName"`

	// The OCID for the volume group.
	Id *string `mandatory:"true" json:"id"`

	// The aggregate size of the volume group in MBs.
	SizeInMBs *int `mandatory:"true" json:"sizeInMBs"`

	// The date and time the volume group was created. Format defined by RFC3339.
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`

	// OCIDs for the volumes in this volume group.
	VolumeIds []string `mandatory:"true" json:"volumeIds"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// The current state of a volume group.
	LifecycleState VolumeGroupLifecycleStateEnum `mandatory:"false" json:"lifecycleState,omitempty"`

	// The aggregate size of the volume group in GBs.
	SizeInGBs *int `mandatory:"false" json:"sizeInGBs"`

	// The volume group source. The source is either another a list of
	// volume IDs in the same availability domain, another volume group, or a volume group backup.
	SourceDetails VolumeGroupSourceDetails `mandatory:"false" json:"sourceDetails"`
}

func (m VolumeGroup) String() string {
	return common.PointerString(m)
}

// UnmarshalJSON unmarshals from json
func (m *VolumeGroup) UnmarshalJSON(data []byte) (e error) {
	model := struct {
		DefinedTags        map[string]map[string]interface{} `json:"definedTags"`
		FreeformTags       map[string]string                 `json:"freeformTags"`
		LifecycleState     VolumeGroupLifecycleStateEnum     `json:"lifecycleState"`
		SizeInGBs          *int                              `json:"sizeInGBs"`
		SourceDetails      volumegroupsourcedetails          `json:"sourceDetails"`
		AvailabilityDomain *string                           `json:"availabilityDomain"`
		CompartmentId      *string                           `json:"compartmentId"`
		DisplayName        *string                           `json:"displayName"`
		Id                 *string                           `json:"id"`
		SizeInMBs          *int                              `json:"sizeInMBs"`
		TimeCreated        *common.SDKTime                   `json:"timeCreated"`
		VolumeIds          []string                          `json:"volumeIds"`
	}{}

	e = json.Unmarshal(data, &model)
	if e != nil {
		return
	}
	m.DefinedTags = model.DefinedTags
	m.FreeformTags = model.FreeformTags
	m.LifecycleState = model.LifecycleState
	m.SizeInGBs = model.SizeInGBs
	nn, e := model.SourceDetails.UnmarshalPolymorphicJSON(model.SourceDetails.JsonData)
	if e != nil {
		return
	}
	m.SourceDetails = nn.(VolumeGroupSourceDetails)
	m.AvailabilityDomain = model.AvailabilityDomain
	m.CompartmentId = model.CompartmentId
	m.DisplayName = model.DisplayName
	m.Id = model.Id
	m.SizeInMBs = model.SizeInMBs
	m.TimeCreated = model.TimeCreated
	m.VolumeIds = make([]string, len(model.VolumeIds))
	for i, n := range model.VolumeIds {
		m.VolumeIds[i] = n
	}
	return
}

// VolumeGroupLifecycleStateEnum Enum with underlying type: string
type VolumeGroupLifecycleStateEnum string

// Set of constants representing the allowable values for VolumeGroupLifecycleState
const (
	VolumeGroupLifecycleStateProvisioning VolumeGroupLifecycleStateEnum = "PROVISIONING"
	VolumeGroupLifecycleStateAvailable    VolumeGroupLifecycleStateEnum = "AVAILABLE"
	VolumeGroupLifecycleStateTerminating  VolumeGroupLifecycleStateEnum = "TERMINATING"
	VolumeGroupLifecycleStateTerminated   VolumeGroupLifecycleStateEnum = "TERMINATED"
	VolumeGroupLifecycleStateFaulty       VolumeGroupLifecycleStateEnum = "FAULTY"
)

var mappingVolumeGroupLifecycleState = map[string]VolumeGroupLifecycleStateEnum{
	"PROVISIONING": VolumeGroupLifecycleStateProvisioning,
	"AVAILABLE":    VolumeGroupLifecycleStateAvailable,
	"TERMINATING":  VolumeGroupLifecycleStateTerminating,
	"TERMINATED":   VolumeGroupLifecycleStateTerminated,
	"FAULTY":       VolumeGroupLifecycleStateFaulty,
}

// GetVolumeGroupLifecycleStateEnumValues Enumerates the set of values for VolumeGroupLifecycleState
func GetVolumeGroupLifecycleStateEnumValues() []VolumeGroupLifecycleStateEnum {
	values := make([]VolumeGroupLifecycleStateEnum, 0)
	for _, v := range mappingVolumeGroupLifecycleState {
		values = append(values, v)
	}
	return values
}
