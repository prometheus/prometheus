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

// DrgAttachment A link between a DRG and VCN. For more information, see
// Overview of the Networking Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/overview.htm).
type DrgAttachment struct {

	// The OCID of the compartment containing the DRG attachment.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The OCID of the DRG.
	DrgId *string `mandatory:"true" json:"drgId"`

	// The DRG attachment's Oracle ID (OCID).
	Id *string `mandatory:"true" json:"id"`

	// The DRG attachment's current state.
	LifecycleState DrgAttachmentLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// The OCID of the VCN.
	VcnId *string `mandatory:"true" json:"vcnId"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// The date and time the DRG attachment was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m DrgAttachment) String() string {
	return common.PointerString(m)
}

// DrgAttachmentLifecycleStateEnum Enum with underlying type: string
type DrgAttachmentLifecycleStateEnum string

// Set of constants representing the allowable values for DrgAttachmentLifecycleState
const (
	DrgAttachmentLifecycleStateAttaching DrgAttachmentLifecycleStateEnum = "ATTACHING"
	DrgAttachmentLifecycleStateAttached  DrgAttachmentLifecycleStateEnum = "ATTACHED"
	DrgAttachmentLifecycleStateDetaching DrgAttachmentLifecycleStateEnum = "DETACHING"
	DrgAttachmentLifecycleStateDetached  DrgAttachmentLifecycleStateEnum = "DETACHED"
)

var mappingDrgAttachmentLifecycleState = map[string]DrgAttachmentLifecycleStateEnum{
	"ATTACHING": DrgAttachmentLifecycleStateAttaching,
	"ATTACHED":  DrgAttachmentLifecycleStateAttached,
	"DETACHING": DrgAttachmentLifecycleStateDetaching,
	"DETACHED":  DrgAttachmentLifecycleStateDetached,
}

// GetDrgAttachmentLifecycleStateEnumValues Enumerates the set of values for DrgAttachmentLifecycleState
func GetDrgAttachmentLifecycleStateEnumValues() []DrgAttachmentLifecycleStateEnum {
	values := make([]DrgAttachmentLifecycleStateEnum, 0)
	for _, v := range mappingDrgAttachmentLifecycleState {
		values = append(values, v)
	}
	return values
}
