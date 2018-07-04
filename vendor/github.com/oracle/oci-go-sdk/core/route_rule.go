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

// RouteRule A mapping between a destination IP address range and a virtual device to route matching
// packets to (a target).
type RouteRule struct {

	// Deprecated, Destination and DestinationType should be used instead; request including both fields will be rejected.
	// A destination IP address range in CIDR notation. Matching packets will
	// be routed to the indicated network entity (the target).
	// Example: `0.0.0.0/0`
	CidrBlock *string `mandatory:"true" json:"cidrBlock"`

	// The OCID for the route rule's target. For information about the type of
	// targets you can specify, see
	// Route Tables (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingroutetables.htm).
	NetworkEntityId *string `mandatory:"true" json:"networkEntityId"`

	// The destination service cidrBlock or destination IP address range in CIDR notation. Matching packets will
	// be routed to the indicated network entity (the target).
	// Examples: `10.12.0.0/16`
	//           `oci-phx-objectstorage`
	Destination *string `mandatory:"false" json:"destination"`

	// Type of destination for the route rule. SERVICE_CIDR_BLOCK should be used if destination is a service
	// cidrBlock. CIDR_BLOCK should be used if destination is IP address range in CIDR notation. It must be provided
	// along with `destination`.
	DestinationType RouteRuleDestinationTypeEnum `mandatory:"false" json:"destinationType,omitempty"`
}

func (m RouteRule) String() string {
	return common.PointerString(m)
}

// RouteRuleDestinationTypeEnum Enum with underlying type: string
type RouteRuleDestinationTypeEnum string

// Set of constants representing the allowable values for RouteRuleDestinationType
const (
	RouteRuleDestinationTypeCidrBlock        RouteRuleDestinationTypeEnum = "CIDR_BLOCK"
	RouteRuleDestinationTypeServiceCidrBlock RouteRuleDestinationTypeEnum = "SERVICE_CIDR_BLOCK"
)

var mappingRouteRuleDestinationType = map[string]RouteRuleDestinationTypeEnum{
	"CIDR_BLOCK":         RouteRuleDestinationTypeCidrBlock,
	"SERVICE_CIDR_BLOCK": RouteRuleDestinationTypeServiceCidrBlock,
}

// GetRouteRuleDestinationTypeEnumValues Enumerates the set of values for RouteRuleDestinationType
func GetRouteRuleDestinationTypeEnumValues() []RouteRuleDestinationTypeEnum {
	values := make([]RouteRuleDestinationTypeEnum, 0)
	for _, v := range mappingRouteRuleDestinationType {
		values = append(values, v)
	}
	return values
}
