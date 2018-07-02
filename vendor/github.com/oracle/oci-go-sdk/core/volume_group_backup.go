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

// VolumeGroupBackup A point-in-time copy of a volume group that can then be used to create a new volume group
// or restore a volume group. For more information, see Volume Groups (https://docs.us-phoenix-1.oraclecloud.com/Content/Block/Concepts/volumegroups.htm).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type VolumeGroupBackup struct {

	// The OCID of the compartment that contains the volume group backup.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// A user-friendly name for the volume group backup. Does not have to be unique and it's changeable. Avoid entering confidential information.
	DisplayName *string `mandatory:"true" json:"displayName"`

	// The OCID of the volume group backup.
	Id *string `mandatory:"true" json:"id"`

	// The current state of a volume group backup.
	LifecycleState VolumeGroupBackupLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// The date and time the volume group backup was created. This is the time the actual point-in-time image
	// of the volume group data was taken. Format defined by RFC3339.
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`

	// The type of backup.
	Type VolumeGroupBackupTypeEnum `mandatory:"true" json:"type"`

	// OCIDs for the volume backups in this volume group backup.
	VolumeBackupIds []string `mandatory:"true" json:"volumeBackupIds"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// The aggregate size of the volume group backup, in MBs.
	SizeInMBs *int `mandatory:"false" json:"sizeInMBs"`

	// The aggregate size of the volume group backup, in GBs.
	SizeInGBs *int `mandatory:"false" json:"sizeInGBs"`

	// The date and time the request to create the volume group backup was received. Format defined by RFC3339.
	TimeRequestReceived *common.SDKTime `mandatory:"false" json:"timeRequestReceived"`

	// The aggregate size used by the volume group backup, in MBs.
	// It is typically smaller than sizeInMBs, depending on the space
	// consumed on the volume group and whether the volume backup is full or incremental.
	UniqueSizeInMbs *int `mandatory:"false" json:"uniqueSizeInMbs"`

	// The aggregate size used by the volume group backup, in GBs.
	// It is typically smaller than sizeInGBs, depending on the space
	// consumed on the volume group and whether the volume backup is full or incremental.
	UniqueSizeInGbs *int `mandatory:"false" json:"uniqueSizeInGbs"`

	// The OCID of the source volume group.
	VolumeGroupId *string `mandatory:"false" json:"volumeGroupId"`
}

func (m VolumeGroupBackup) String() string {
	return common.PointerString(m)
}

// VolumeGroupBackupLifecycleStateEnum Enum with underlying type: string
type VolumeGroupBackupLifecycleStateEnum string

// Set of constants representing the allowable values for VolumeGroupBackupLifecycleState
const (
	VolumeGroupBackupLifecycleStateCreating        VolumeGroupBackupLifecycleStateEnum = "CREATING"
	VolumeGroupBackupLifecycleStateCommitted       VolumeGroupBackupLifecycleStateEnum = "COMMITTED"
	VolumeGroupBackupLifecycleStateAvailable       VolumeGroupBackupLifecycleStateEnum = "AVAILABLE"
	VolumeGroupBackupLifecycleStateTerminating     VolumeGroupBackupLifecycleStateEnum = "TERMINATING"
	VolumeGroupBackupLifecycleStateTerminated      VolumeGroupBackupLifecycleStateEnum = "TERMINATED"
	VolumeGroupBackupLifecycleStateFaulty          VolumeGroupBackupLifecycleStateEnum = "FAULTY"
	VolumeGroupBackupLifecycleStateRequestReceived VolumeGroupBackupLifecycleStateEnum = "REQUEST_RECEIVED"
)

var mappingVolumeGroupBackupLifecycleState = map[string]VolumeGroupBackupLifecycleStateEnum{
	"CREATING":         VolumeGroupBackupLifecycleStateCreating,
	"COMMITTED":        VolumeGroupBackupLifecycleStateCommitted,
	"AVAILABLE":        VolumeGroupBackupLifecycleStateAvailable,
	"TERMINATING":      VolumeGroupBackupLifecycleStateTerminating,
	"TERMINATED":       VolumeGroupBackupLifecycleStateTerminated,
	"FAULTY":           VolumeGroupBackupLifecycleStateFaulty,
	"REQUEST_RECEIVED": VolumeGroupBackupLifecycleStateRequestReceived,
}

// GetVolumeGroupBackupLifecycleStateEnumValues Enumerates the set of values for VolumeGroupBackupLifecycleState
func GetVolumeGroupBackupLifecycleStateEnumValues() []VolumeGroupBackupLifecycleStateEnum {
	values := make([]VolumeGroupBackupLifecycleStateEnum, 0)
	for _, v := range mappingVolumeGroupBackupLifecycleState {
		values = append(values, v)
	}
	return values
}

// VolumeGroupBackupTypeEnum Enum with underlying type: string
type VolumeGroupBackupTypeEnum string

// Set of constants representing the allowable values for VolumeGroupBackupType
const (
	VolumeGroupBackupTypeFull        VolumeGroupBackupTypeEnum = "FULL"
	VolumeGroupBackupTypeIncremental VolumeGroupBackupTypeEnum = "INCREMENTAL"
)

var mappingVolumeGroupBackupType = map[string]VolumeGroupBackupTypeEnum{
	"FULL":        VolumeGroupBackupTypeFull,
	"INCREMENTAL": VolumeGroupBackupTypeIncremental,
}

// GetVolumeGroupBackupTypeEnumValues Enumerates the set of values for VolumeGroupBackupType
func GetVolumeGroupBackupTypeEnumValues() []VolumeGroupBackupTypeEnum {
	values := make([]VolumeGroupBackupTypeEnum, 0)
	for _, v := range mappingVolumeGroupBackupType {
		values = append(values, v)
	}
	return values
}
