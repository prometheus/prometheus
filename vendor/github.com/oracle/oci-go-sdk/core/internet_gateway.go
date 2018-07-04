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

// InternetGateway Represents a router that connects the edge of a VCN with the Internet. For an example scenario
// that uses an Internet Gateway, see
// Typical Networking Service Scenarios (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/overview.htm#scenarios).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type InternetGateway struct {

	// The OCID of the compartment containing the Internet Gateway.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The Internet Gateway's Oracle ID (OCID).
	Id *string `mandatory:"true" json:"id"`

	// The Internet Gateway's current state.
	LifecycleState InternetGatewayLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// The OCID of the VCN the Internet Gateway belongs to.
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

	// Whether the gateway is enabled. When the gateway is disabled, traffic is not
	// routed to/from the Internet, regardless of route rules.
	IsEnabled *bool `mandatory:"false" json:"isEnabled"`

	// The date and time the Internet Gateway was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m InternetGateway) String() string {
	return common.PointerString(m)
}

// InternetGatewayLifecycleStateEnum Enum with underlying type: string
type InternetGatewayLifecycleStateEnum string

// Set of constants representing the allowable values for InternetGatewayLifecycleState
const (
	InternetGatewayLifecycleStateProvisioning InternetGatewayLifecycleStateEnum = "PROVISIONING"
	InternetGatewayLifecycleStateAvailable    InternetGatewayLifecycleStateEnum = "AVAILABLE"
	InternetGatewayLifecycleStateTerminating  InternetGatewayLifecycleStateEnum = "TERMINATING"
	InternetGatewayLifecycleStateTerminated   InternetGatewayLifecycleStateEnum = "TERMINATED"
)

var mappingInternetGatewayLifecycleState = map[string]InternetGatewayLifecycleStateEnum{
	"PROVISIONING": InternetGatewayLifecycleStateProvisioning,
	"AVAILABLE":    InternetGatewayLifecycleStateAvailable,
	"TERMINATING":  InternetGatewayLifecycleStateTerminating,
	"TERMINATED":   InternetGatewayLifecycleStateTerminated,
}

// GetInternetGatewayLifecycleStateEnumValues Enumerates the set of values for InternetGatewayLifecycleState
func GetInternetGatewayLifecycleStateEnumValues() []InternetGatewayLifecycleStateEnum {
	values := make([]InternetGatewayLifecycleStateEnum, 0)
	for _, v := range mappingInternetGatewayLifecycleState {
		values = append(values, v)
	}
	return values
}
