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

// CreateBootVolumeBackupDetails The representation of CreateBootVolumeBackupDetails
type CreateBootVolumeBackupDetails struct {

	// The OCID of the boot volume that needs to be backed up.
	BootVolumeId *string `mandatory:"true" json:"bootVolumeId"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name for the boot volume backup. Does not have to be unique and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// The type of backup to create. If omitted, defaults to incremental.
	Type CreateBootVolumeBackupDetailsTypeEnum `mandatory:"false" json:"type,omitempty"`
}

func (m CreateBootVolumeBackupDetails) String() string {
	return common.PointerString(m)
}

// CreateBootVolumeBackupDetailsTypeEnum Enum with underlying type: string
type CreateBootVolumeBackupDetailsTypeEnum string

// Set of constants representing the allowable values for CreateBootVolumeBackupDetailsType
const (
	CreateBootVolumeBackupDetailsTypeFull        CreateBootVolumeBackupDetailsTypeEnum = "FULL"
	CreateBootVolumeBackupDetailsTypeIncremental CreateBootVolumeBackupDetailsTypeEnum = "INCREMENTAL"
)

var mappingCreateBootVolumeBackupDetailsType = map[string]CreateBootVolumeBackupDetailsTypeEnum{
	"FULL":        CreateBootVolumeBackupDetailsTypeFull,
	"INCREMENTAL": CreateBootVolumeBackupDetailsTypeIncremental,
}

// GetCreateBootVolumeBackupDetailsTypeEnumValues Enumerates the set of values for CreateBootVolumeBackupDetailsType
func GetCreateBootVolumeBackupDetailsTypeEnumValues() []CreateBootVolumeBackupDetailsTypeEnum {
	values := make([]CreateBootVolumeBackupDetailsTypeEnum, 0)
	for _, v := range mappingCreateBootVolumeBackupDetailsType {
		values = append(values, v)
	}
	return values
}
