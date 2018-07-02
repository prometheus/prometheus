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

// UpdateServiceGatewayDetails The representation of UpdateServiceGatewayDetails
type UpdateServiceGatewayDetails struct {

	// Whether the service gateway blocks all traffic through it. The default is `false`. When
	// this is `true`, traffic is not routed to any services, regardless of route rules.
	// Example: `true`
	BlockTraffic *bool `mandatory:"false" json:"blockTraffic"`

	// Usage of predefined tag keys. These predefined keys are scoped to namespaces.
	// Example: `{"foo-namespace": {"bar-key": "foo-value"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Simple key-value pair that is applied without any predefined name, type or scope. Exists for cross-compatibility only.
	// Example: `{"bar-key": "value"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// List of all the services you want enabled on this service gateway. Sending an empty list
	// means you want to disable all services. Omitting this parameter entirely keeps the
	// existing list of services intact.
	// You can also enable or disable a particular service by using
	// AttachServiceId and
	// DetachServiceId.
	// For each enabled service, make sure there's a route rule with the service's `cidrBlock`
	// as the rule's destination CIDR and the service gateway as the rule's target. See
	// RouteTable.
	Services []ServiceIdRequestDetails `mandatory:"false" json:"services"`
}

func (m UpdateServiceGatewayDetails) String() string {
	return common.PointerString(m)
}
