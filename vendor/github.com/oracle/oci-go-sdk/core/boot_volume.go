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

// BootVolume A detachable boot volume device that contains the image used to boot a Compute instance. For more information, see
// Overview of Boot Volumes (https://docs.us-phoenix-1.oraclecloud.com/Content/Block/Concepts/bootvolumes.htm).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type BootVolume struct {

	// The Availability Domain of the boot volume.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"true" json:"availabilityDomain"`

	// The OCID of the compartment that contains the boot volume.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The boot volume's Oracle ID (OCID).
	Id *string `mandatory:"true" json:"id"`

	// The current state of a boot volume.
	LifecycleState BootVolumeLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// The size of the volume in MBs. The value must be a multiple of 1024.
	// This field is deprecated. Please use sizeInGBs.
	SizeInMBs *int `mandatory:"true" json:"sizeInMBs"`

	// The date and time the boot volume was created. Format defined by RFC3339.
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// The image OCID used to create the boot volume.
	ImageId *string `mandatory:"false" json:"imageId"`

	// Specifies whether the boot volume's data has finished copying from the source boot volume or boot volume backup.
	IsHydrated *bool `mandatory:"false" json:"isHydrated"`

	// The size of the boot volume in GBs.
	SizeInGBs *int `mandatory:"false" json:"sizeInGBs"`

	// The boot volume source, either an existing boot volume in the same Availability Domain or a boot volume backup.
	// If null, this means that the boot volume was created from an image.
	SourceDetails BootVolumeSourceDetails `mandatory:"false" json:"sourceDetails"`

	// The OCID of the source volume group.
	VolumeGroupId *string `mandatory:"false" json:"volumeGroupId"`
}

func (m BootVolume) String() string {
	return common.PointerString(m)
}

// UnmarshalJSON unmarshals from json
func (m *BootVolume) UnmarshalJSON(data []byte) (e error) {
	model := struct {
		DefinedTags        map[string]map[string]interface{} `json:"definedTags"`
		DisplayName        *string                           `json:"displayName"`
		FreeformTags       map[string]string                 `json:"freeformTags"`
		ImageId            *string                           `json:"imageId"`
		IsHydrated         *bool                             `json:"isHydrated"`
		SizeInGBs          *int                              `json:"sizeInGBs"`
		SourceDetails      bootvolumesourcedetails           `json:"sourceDetails"`
		VolumeGroupId      *string                           `json:"volumeGroupId"`
		AvailabilityDomain *string                           `json:"availabilityDomain"`
		CompartmentId      *string                           `json:"compartmentId"`
		Id                 *string                           `json:"id"`
		LifecycleState     BootVolumeLifecycleStateEnum      `json:"lifecycleState"`
		SizeInMBs          *int                              `json:"sizeInMBs"`
		TimeCreated        *common.SDKTime                   `json:"timeCreated"`
	}{}

	e = json.Unmarshal(data, &model)
	if e != nil {
		return
	}
	m.DefinedTags = model.DefinedTags
	m.DisplayName = model.DisplayName
	m.FreeformTags = model.FreeformTags
	m.ImageId = model.ImageId
	m.IsHydrated = model.IsHydrated
	m.SizeInGBs = model.SizeInGBs
	nn, e := model.SourceDetails.UnmarshalPolymorphicJSON(model.SourceDetails.JsonData)
	if e != nil {
		return
	}
	m.SourceDetails = nn.(BootVolumeSourceDetails)
	m.VolumeGroupId = model.VolumeGroupId
	m.AvailabilityDomain = model.AvailabilityDomain
	m.CompartmentId = model.CompartmentId
	m.Id = model.Id
	m.LifecycleState = model.LifecycleState
	m.SizeInMBs = model.SizeInMBs
	m.TimeCreated = model.TimeCreated
	return
}

// BootVolumeLifecycleStateEnum Enum with underlying type: string
type BootVolumeLifecycleStateEnum string

// Set of constants representing the allowable values for BootVolumeLifecycleState
const (
	BootVolumeLifecycleStateProvisioning BootVolumeLifecycleStateEnum = "PROVISIONING"
	BootVolumeLifecycleStateRestoring    BootVolumeLifecycleStateEnum = "RESTORING"
	BootVolumeLifecycleStateAvailable    BootVolumeLifecycleStateEnum = "AVAILABLE"
	BootVolumeLifecycleStateTerminating  BootVolumeLifecycleStateEnum = "TERMINATING"
	BootVolumeLifecycleStateTerminated   BootVolumeLifecycleStateEnum = "TERMINATED"
	BootVolumeLifecycleStateFaulty       BootVolumeLifecycleStateEnum = "FAULTY"
)

var mappingBootVolumeLifecycleState = map[string]BootVolumeLifecycleStateEnum{
	"PROVISIONING": BootVolumeLifecycleStateProvisioning,
	"RESTORING":    BootVolumeLifecycleStateRestoring,
	"AVAILABLE":    BootVolumeLifecycleStateAvailable,
	"TERMINATING":  BootVolumeLifecycleStateTerminating,
	"TERMINATED":   BootVolumeLifecycleStateTerminated,
	"FAULTY":       BootVolumeLifecycleStateFaulty,
}

// GetBootVolumeLifecycleStateEnumValues Enumerates the set of values for BootVolumeLifecycleState
func GetBootVolumeLifecycleStateEnumValues() []BootVolumeLifecycleStateEnum {
	values := make([]BootVolumeLifecycleStateEnum, 0)
	for _, v := range mappingBootVolumeLifecycleState {
		values = append(values, v)
	}
	return values
}
