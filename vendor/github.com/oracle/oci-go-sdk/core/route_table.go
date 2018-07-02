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

// RouteTable A collection of `RouteRule` objects, which are used to route packets
// based on destination IP to a particular network entity. For more information, see
// Overview of the Networking Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/overview.htm).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type RouteTable struct {

	// The OCID of the compartment containing the route table.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The route table's Oracle ID (OCID).
	Id *string `mandatory:"true" json:"id"`

	// The route table's current state.
	LifecycleState RouteTableLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// The collection of rules for routing destination IPs to network devices.
	RouteRules []RouteRule `mandatory:"true" json:"routeRules"`

	// The OCID of the VCN the route table list belongs to.
	VcnId *string `mandatory:"true" json:"vcnId"`

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

	// The date and time the route table was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m RouteTable) String() string {
	return common.PointerString(m)
}

// RouteTableLifecycleStateEnum Enum with underlying type: string
type RouteTableLifecycleStateEnum string

// Set of constants representing the allowable values for RouteTableLifecycleState
const (
	RouteTableLifecycleStateProvisioning RouteTableLifecycleStateEnum = "PROVISIONING"
	RouteTableLifecycleStateAvailable    RouteTableLifecycleStateEnum = "AVAILABLE"
	RouteTableLifecycleStateTerminating  RouteTableLifecycleStateEnum = "TERMINATING"
	RouteTableLifecycleStateTerminated   RouteTableLifecycleStateEnum = "TERMINATED"
)

var mappingRouteTableLifecycleState = map[string]RouteTableLifecycleStateEnum{
	"PROVISIONING": RouteTableLifecycleStateProvisioning,
	"AVAILABLE":    RouteTableLifecycleStateAvailable,
	"TERMINATING":  RouteTableLifecycleStateTerminating,
	"TERMINATED":   RouteTableLifecycleStateTerminated,
}

// GetRouteTableLifecycleStateEnumValues Enumerates the set of values for RouteTableLifecycleState
func GetRouteTableLifecycleStateEnumValues() []RouteTableLifecycleStateEnum {
	values := make([]RouteTableLifecycleStateEnum, 0)
	for _, v := range mappingRouteTableLifecycleState {
		values = append(values, v)
	}
	return values
}
