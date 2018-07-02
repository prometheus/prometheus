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

// ServiceGateway Represents a router that connects the edge of a VCN with public Oracle Cloud Infrastructure
// services such as Object Storage. Traffic leaving the VCN and destined for a supported public
// service (see ListServices) is routed through the
// service gateway and does not traverse the internet. The instances in the VCN do not need to
// have public IP addresses nor be in a public subnet. The VCN does not need an internet gateway
// for this traffic. For more information, see
// Access to Object Storage: Service Gateway (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/servicegateway.htm).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type ServiceGateway struct {

	// Whether the service gateway blocks all traffic through it. The default is `false`. When
	// this is `true`, traffic is not routed to any services, regardless of route rules.
	// Example: `true`
	BlockTraffic *bool `mandatory:"true" json:"blockTraffic"`

	// The OCID (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm) of the compartment that contains the
	// service gateway.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The OCID (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm) of the service gateway.
	Id *string `mandatory:"true" json:"id"`

	// The service gateway's current state.
	LifecycleState ServiceGatewayLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// List of the services enabled on this service gateway. The list can be empty.
	// You can enable a particular service by using
	// AttachServiceId.
	Services []ServiceIdResponseDetails `mandatory:"true" json:"services"`

	// The OCID (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm) of the VCN the service gateway
	// belongs to.
	VcnId *string `mandatory:"true" json:"vcnId"`

	// Usage of predefined tag keys. These predefined keys are scoped to namespaces.
	// Example: `{"foo-namespace": {"bar-key": "foo-value"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Simple key-value pair that is applied without any predefined name, type or scope. Exists for cross-compatibility only.
	// Example: `{"bar-key": "value"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// The date and time the service gateway was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m ServiceGateway) String() string {
	return common.PointerString(m)
}

// ServiceGatewayLifecycleStateEnum Enum with underlying type: string
type ServiceGatewayLifecycleStateEnum string

// Set of constants representing the allowable values for ServiceGatewayLifecycleState
const (
	ServiceGatewayLifecycleStateProvisioning ServiceGatewayLifecycleStateEnum = "PROVISIONING"
	ServiceGatewayLifecycleStateAvailable    ServiceGatewayLifecycleStateEnum = "AVAILABLE"
	ServiceGatewayLifecycleStateTerminating  ServiceGatewayLifecycleStateEnum = "TERMINATING"
	ServiceGatewayLifecycleStateTerminated   ServiceGatewayLifecycleStateEnum = "TERMINATED"
)

var mappingServiceGatewayLifecycleState = map[string]ServiceGatewayLifecycleStateEnum{
	"PROVISIONING": ServiceGatewayLifecycleStateProvisioning,
	"AVAILABLE":    ServiceGatewayLifecycleStateAvailable,
	"TERMINATING":  ServiceGatewayLifecycleStateTerminating,
	"TERMINATED":   ServiceGatewayLifecycleStateTerminated,
}

// GetServiceGatewayLifecycleStateEnumValues Enumerates the set of values for ServiceGatewayLifecycleState
func GetServiceGatewayLifecycleStateEnumValues() []ServiceGatewayLifecycleStateEnum {
	values := make([]ServiceGatewayLifecycleStateEnum, 0)
	for _, v := range mappingServiceGatewayLifecycleState {
		values = append(values, v)
	}
	return values
}
