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

// CrossConnect For use with Oracle Cloud Infrastructure FastConnect. A cross-connect represents a
// physical connection between an existing network and Oracle. Customers who are colocated
// with Oracle in a FastConnect location create and use cross-connects. For more
// information, see FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
// Oracle recommends you create each cross-connect in a
// CrossConnectGroup so you can use link aggregation
// with the connection.
// **Note:** If you're a provider who is setting up a physical connection to Oracle so customers
// can use FastConnect over the connection, be aware that your connection is modeled the
// same way as a colocated customer's (with `CrossConnect` and `CrossConnectGroup` objects, and so on).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type CrossConnect struct {

	// The OCID of the compartment containing the cross-connect group.
	CompartmentId *string `mandatory:"false" json:"compartmentId"`

	// The OCID of the cross-connect group this cross-connect belongs to (if any).
	CrossConnectGroupId *string `mandatory:"false" json:"crossConnectGroupId"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// The cross-connect's Oracle ID (OCID).
	Id *string `mandatory:"false" json:"id"`

	// The cross-connect's current state.
	LifecycleState CrossConnectLifecycleStateEnum `mandatory:"false" json:"lifecycleState,omitempty"`

	// The name of the FastConnect location where this cross-connect is installed.
	LocationName *string `mandatory:"false" json:"locationName"`

	// A string identifying the meet-me room port for this cross-connect.
	PortName *string `mandatory:"false" json:"portName"`

	// The port speed for this cross-connect.
	// Example: `10 Gbps`
	PortSpeedShapeName *string `mandatory:"false" json:"portSpeedShapeName"`

	// The date and time the cross-connect was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m CrossConnect) String() string {
	return common.PointerString(m)
}

// CrossConnectLifecycleStateEnum Enum with underlying type: string
type CrossConnectLifecycleStateEnum string

// Set of constants representing the allowable values for CrossConnectLifecycleState
const (
	CrossConnectLifecycleStatePendingCustomer CrossConnectLifecycleStateEnum = "PENDING_CUSTOMER"
	CrossConnectLifecycleStateProvisioning    CrossConnectLifecycleStateEnum = "PROVISIONING"
	CrossConnectLifecycleStateProvisioned     CrossConnectLifecycleStateEnum = "PROVISIONED"
	CrossConnectLifecycleStateInactive        CrossConnectLifecycleStateEnum = "INACTIVE"
	CrossConnectLifecycleStateTerminating     CrossConnectLifecycleStateEnum = "TERMINATING"
	CrossConnectLifecycleStateTerminated      CrossConnectLifecycleStateEnum = "TERMINATED"
)

var mappingCrossConnectLifecycleState = map[string]CrossConnectLifecycleStateEnum{
	"PENDING_CUSTOMER": CrossConnectLifecycleStatePendingCustomer,
	"PROVISIONING":     CrossConnectLifecycleStateProvisioning,
	"PROVISIONED":      CrossConnectLifecycleStateProvisioned,
	"INACTIVE":         CrossConnectLifecycleStateInactive,
	"TERMINATING":      CrossConnectLifecycleStateTerminating,
	"TERMINATED":       CrossConnectLifecycleStateTerminated,
}

// GetCrossConnectLifecycleStateEnumValues Enumerates the set of values for CrossConnectLifecycleState
func GetCrossConnectLifecycleStateEnumValues() []CrossConnectLifecycleStateEnum {
	values := make([]CrossConnectLifecycleStateEnum, 0)
	for _, v := range mappingCrossConnectLifecycleState {
		values = append(values, v)
	}
	return values
}
