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

// CrossConnectGroup For use with Oracle Cloud Infrastructure FastConnect. A cross-connect group
// is a link aggregation group (LAG), which can contain one or more
// CrossConnect. Customers who are colocated with
// Oracle in a FastConnect location create and use cross-connect groups. For more
// information, see FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
// **Note:** If you're a provider who is setting up a physical connection to Oracle so customers
// can use FastConnect over the connection, be aware that your connection is modeled the
// same way as a colocated customer's (with `CrossConnect` and `CrossConnectGroup` objects, and so on).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type CrossConnectGroup struct {

	// The OCID of the compartment containing the cross-connect group.
	CompartmentId *string `mandatory:"false" json:"compartmentId"`

	// The display name of A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// The cross-connect group's Oracle ID (OCID).
	Id *string `mandatory:"false" json:"id"`

	// The cross-connect group's current state.
	LifecycleState CrossConnectGroupLifecycleStateEnum `mandatory:"false" json:"lifecycleState,omitempty"`

	// The date and time the cross-connect group was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m CrossConnectGroup) String() string {
	return common.PointerString(m)
}

// CrossConnectGroupLifecycleStateEnum Enum with underlying type: string
type CrossConnectGroupLifecycleStateEnum string

// Set of constants representing the allowable values for CrossConnectGroupLifecycleState
const (
	CrossConnectGroupLifecycleStateProvisioning CrossConnectGroupLifecycleStateEnum = "PROVISIONING"
	CrossConnectGroupLifecycleStateProvisioned  CrossConnectGroupLifecycleStateEnum = "PROVISIONED"
	CrossConnectGroupLifecycleStateInactive     CrossConnectGroupLifecycleStateEnum = "INACTIVE"
	CrossConnectGroupLifecycleStateTerminating  CrossConnectGroupLifecycleStateEnum = "TERMINATING"
	CrossConnectGroupLifecycleStateTerminated   CrossConnectGroupLifecycleStateEnum = "TERMINATED"
)

var mappingCrossConnectGroupLifecycleState = map[string]CrossConnectGroupLifecycleStateEnum{
	"PROVISIONING": CrossConnectGroupLifecycleStateProvisioning,
	"PROVISIONED":  CrossConnectGroupLifecycleStateProvisioned,
	"INACTIVE":     CrossConnectGroupLifecycleStateInactive,
	"TERMINATING":  CrossConnectGroupLifecycleStateTerminating,
	"TERMINATED":   CrossConnectGroupLifecycleStateTerminated,
}

// GetCrossConnectGroupLifecycleStateEnumValues Enumerates the set of values for CrossConnectGroupLifecycleState
func GetCrossConnectGroupLifecycleStateEnumValues() []CrossConnectGroupLifecycleStateEnum {
	values := make([]CrossConnectGroupLifecycleStateEnum, 0)
	for _, v := range mappingCrossConnectGroupLifecycleState {
		values = append(values, v)
	}
	return values
}
