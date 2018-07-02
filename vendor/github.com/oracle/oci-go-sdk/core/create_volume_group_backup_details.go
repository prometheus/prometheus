// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

// Core Services API
//
// APIs for Networking Service, Compute Service, and Block Volume Service.
//

package core

import (
	"github.com/oracle/oci-go-sdk/common"
)

// CreateVolumeGroupBackupDetails The representation of CreateVolumeGroupBackupDetails
type CreateVolumeGroupBackupDetails struct {

	// The OCID of the volume group that needs to be backed up.
	VolumeGroupId *string `mandatory:"true" json:"volumeGroupId"`

	// The OCID of the compartment that will contain the volume group backup. This parameter is optional, by default backup will be created in the same compartment and source volume group.
	CompartmentId *string `mandatory:"false" json:"compartmentId"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name for the volume group backup. Does not have to be unique and it's changeable.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// The type of backup to create. If omitted, defaults to incremental.
	Type CreateVolumeGroupBackupDetailsTypeEnum `mandatory:"false" json:"type,omitempty"`
}

func (m CreateVolumeGroupBackupDetails) String() string {
	return common.PointerString(m)
}

// CreateVolumeGroupBackupDetailsTypeEnum Enum with underlying type: string
type CreateVolumeGroupBackupDetailsTypeEnum string

// Set of constants representing the allowable values for CreateVolumeGroupBackupDetailsType
const (
	CreateVolumeGroupBackupDetailsTypeFull        CreateVolumeGroupBackupDetailsTypeEnum = "FULL"
	CreateVolumeGroupBackupDetailsTypeIncremental CreateVolumeGroupBackupDetailsTypeEnum = "INCREMENTAL"
)

var mappingCreateVolumeGroupBackupDetailsType = map[string]CreateVolumeGroupBackupDetailsTypeEnum{
	"FULL":        CreateVolumeGroupBackupDetailsTypeFull,
	"INCREMENTAL": CreateVolumeGroupBackupDetailsTypeIncremental,
}

// GetCreateVolumeGroupBackupDetailsTypeEnumValues Enumerates the set of values for CreateVolumeGroupBackupDetailsType
func GetCreateVolumeGroupBackupDetailsTypeEnumValues() []CreateVolumeGroupBackupDetailsTypeEnum {
	values := make([]CreateVolumeGroupBackupDetailsTypeEnum, 0)
	for _, v := range mappingCreateVolumeGroupBackupDetailsType {
		values = append(values, v)
	}
	return values
}
