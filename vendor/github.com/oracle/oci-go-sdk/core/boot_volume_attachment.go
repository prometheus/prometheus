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

// BootVolumeAttachment Represents an attachment between a boot volume and an instance.
type BootVolumeAttachment struct {

	// The Availability Domain of an instance.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"true" json:"availabilityDomain"`

	// The OCID of the boot volume.
	BootVolumeId *string `mandatory:"true" json:"bootVolumeId"`

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The OCID of the boot volume attachment.
	Id *string `mandatory:"true" json:"id"`

	// The OCID of the instance the boot volume is attached to.
	InstanceId *string `mandatory:"true" json:"instanceId"`

	// The current state of the boot volume attachment.
	LifecycleState BootVolumeAttachmentLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// The date and time the boot volume was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`

	// A user-friendly name. Does not have to be unique, and it cannot be changed.
	// Avoid entering confidential information.
	// Example: `My boot volume`
	DisplayName *string `mandatory:"false" json:"displayName"`
}

func (m BootVolumeAttachment) String() string {
	return common.PointerString(m)
}

// BootVolumeAttachmentLifecycleStateEnum Enum with underlying type: string
type BootVolumeAttachmentLifecycleStateEnum string

// Set of constants representing the allowable values for BootVolumeAttachmentLifecycleState
const (
	BootVolumeAttachmentLifecycleStateAttaching BootVolumeAttachmentLifecycleStateEnum = "ATTACHING"
	BootVolumeAttachmentLifecycleStateAttached  BootVolumeAttachmentLifecycleStateEnum = "ATTACHED"
	BootVolumeAttachmentLifecycleStateDetaching BootVolumeAttachmentLifecycleStateEnum = "DETACHING"
	BootVolumeAttachmentLifecycleStateDetached  BootVolumeAttachmentLifecycleStateEnum = "DETACHED"
)

var mappingBootVolumeAttachmentLifecycleState = map[string]BootVolumeAttachmentLifecycleStateEnum{
	"ATTACHING": BootVolumeAttachmentLifecycleStateAttaching,
	"ATTACHED":  BootVolumeAttachmentLifecycleStateAttached,
	"DETACHING": BootVolumeAttachmentLifecycleStateDetaching,
	"DETACHED":  BootVolumeAttachmentLifecycleStateDetached,
}

// GetBootVolumeAttachmentLifecycleStateEnumValues Enumerates the set of values for BootVolumeAttachmentLifecycleState
func GetBootVolumeAttachmentLifecycleStateEnumValues() []BootVolumeAttachmentLifecycleStateEnum {
	values := make([]BootVolumeAttachmentLifecycleStateEnum, 0)
	for _, v := range mappingBootVolumeAttachmentLifecycleState {
		values = append(values, v)
	}
	return values
}
