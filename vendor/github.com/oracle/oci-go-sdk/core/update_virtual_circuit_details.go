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

// UpdateVirtualCircuitDetails The representation of UpdateVirtualCircuitDetails
type UpdateVirtualCircuitDetails struct {

	// The provisioned data rate of the connection. To get a list of the
	// available bandwidth levels (that is, shapes), see
	// ListFastConnectProviderVirtualCircuitBandwidthShapes.
	// To be updated only by the customer who owns the virtual circuit.
	BandwidthShapeName *string `mandatory:"false" json:"bandwidthShapeName"`

	// An array of mappings, each containing properties for a cross-connect or
	// cross-connect group associated with this virtual circuit.
	// The customer and provider can update different properties in the mapping
	// depending on the situation. See the description of the
	// CrossConnectMapping.
	CrossConnectMappings []CrossConnectMapping `mandatory:"false" json:"crossConnectMappings"`

	// The BGP ASN of the network at the other end of the BGP
	// session from Oracle.
	// If the BGP session is from the customer's edge router to Oracle, the
	// required value is the customer's ASN, and it can be updated only
	// by the customer.
	// If the BGP session is from the provider's edge router to Oracle, the
	// required value is the provider's ASN, and it can be updated only
	// by the provider.
	CustomerBgpAsn *int `mandatory:"false" json:"customerBgpAsn"`

	// A user-friendly name. Does not have to be unique.
	// Avoid entering confidential information.
	// To be updated only by the customer who owns the virtual circuit.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// The OCID of the Drg
	// that this private virtual circuit uses.
	// To be updated only by the customer who owns the virtual circuit.
	GatewayId *string `mandatory:"false" json:"gatewayId"`

	// The provider's state in relation to this virtual circuit. Relevant only
	// if the customer is using FastConnect via a provider.  ACTIVE
	// means the provider has provisioned the virtual circuit from their
	// end. INACTIVE means the provider has not yet provisioned the virtual
	// circuit, or has de-provisioned it.
	// To be updated only by the provider.
	ProviderState UpdateVirtualCircuitDetailsProviderStateEnum `mandatory:"false" json:"providerState,omitempty"`

	// Provider-supplied reference information about this virtual circuit.
	// Relevant only if the customer is using FastConnect via a provider.
	// To be updated only by the provider.
	ReferenceComment *string `mandatory:"false" json:"referenceComment"`
}

func (m UpdateVirtualCircuitDetails) String() string {
	return common.PointerString(m)
}

// UpdateVirtualCircuitDetailsProviderStateEnum Enum with underlying type: string
type UpdateVirtualCircuitDetailsProviderStateEnum string

// Set of constants representing the allowable values for UpdateVirtualCircuitDetailsProviderState
const (
	UpdateVirtualCircuitDetailsProviderStateActive   UpdateVirtualCircuitDetailsProviderStateEnum = "ACTIVE"
	UpdateVirtualCircuitDetailsProviderStateInactive UpdateVirtualCircuitDetailsProviderStateEnum = "INACTIVE"
)

var mappingUpdateVirtualCircuitDetailsProviderState = map[string]UpdateVirtualCircuitDetailsProviderStateEnum{
	"ACTIVE":   UpdateVirtualCircuitDetailsProviderStateActive,
	"INACTIVE": UpdateVirtualCircuitDetailsProviderStateInactive,
}

// GetUpdateVirtualCircuitDetailsProviderStateEnumValues Enumerates the set of values for UpdateVirtualCircuitDetailsProviderState
func GetUpdateVirtualCircuitDetailsProviderStateEnumValues() []UpdateVirtualCircuitDetailsProviderStateEnum {
	values := make([]UpdateVirtualCircuitDetailsProviderStateEnum, 0)
	for _, v := range mappingUpdateVirtualCircuitDetailsProviderState {
		values = append(values, v)
	}
	return values
}
