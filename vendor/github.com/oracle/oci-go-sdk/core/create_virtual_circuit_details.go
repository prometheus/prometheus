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

// CreateVirtualCircuitDetails The representation of CreateVirtualCircuitDetails
type CreateVirtualCircuitDetails struct {

	// The OCID of the compartment to contain the virtual circuit.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The type of IP addresses used in this virtual circuit. PRIVATE
	// means RFC 1918 (https://tools.ietf.org/html/rfc1918) addresses
	// (10.0.0.0/8, 172.16/12, and 192.168/16). Only PRIVATE is supported.
	Type CreateVirtualCircuitDetailsTypeEnum `mandatory:"true" json:"type"`

	// The provisioned data rate of the connection.  To get a list of the
	// available bandwidth levels (that is, shapes), see
	// ListFastConnectProviderVirtualCircuitBandwidthShapes.
	// Example: `10 Gbps`
	BandwidthShapeName *string `mandatory:"false" json:"bandwidthShapeName"`

	// Create a `CrossConnectMapping` for each cross-connect or cross-connect
	// group this virtual circuit will run on.
	CrossConnectMappings []CrossConnectMapping `mandatory:"false" json:"crossConnectMappings"`

	// Your BGP ASN (either public or private). Provide this value only if
	// there's a BGP session that goes from your edge router to Oracle.
	// Otherwise, leave this empty or null.
	CustomerBgpAsn *int `mandatory:"false" json:"customerBgpAsn"`

	// A user-friendly name. Does not have to be unique, and it's changeable. Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// For private virtual circuits only. The OCID of the Drg
	// that this virtual circuit uses.
	GatewayId *string `mandatory:"false" json:"gatewayId"`

	// Deprecated. Instead use `providerServiceId`.
	// To get a list of the provider names, see
	// ListFastConnectProviderServices.
	ProviderName *string `mandatory:"false" json:"providerName"`

	// The OCID of the service offered by the provider (if you're connecting
	// via a provider). To get a list of the available service offerings, see
	// ListFastConnectProviderServices.
	ProviderServiceId *string `mandatory:"false" json:"providerServiceId"`

	// Deprecated. Instead use `providerServiceId`.
	// To get a list of the provider names, see
	// ListFastConnectProviderServices.
	ProviderServiceName *string `mandatory:"false" json:"providerServiceName"`

	// For a public virtual circuit. The public IP prefixes (CIDRs) the customer wants to
	// advertise across the connection.
	PublicPrefixes []CreateVirtualCircuitPublicPrefixDetails `mandatory:"false" json:"publicPrefixes"`

	// The Oracle Cloud Infrastructure region where this virtual
	// circuit is located.
	// Example: `phx`
	Region *string `mandatory:"false" json:"region"`
}

func (m CreateVirtualCircuitDetails) String() string {
	return common.PointerString(m)
}

// CreateVirtualCircuitDetailsTypeEnum Enum with underlying type: string
type CreateVirtualCircuitDetailsTypeEnum string

// Set of constants representing the allowable values for CreateVirtualCircuitDetailsType
const (
	CreateVirtualCircuitDetailsTypePublic  CreateVirtualCircuitDetailsTypeEnum = "PUBLIC"
	CreateVirtualCircuitDetailsTypePrivate CreateVirtualCircuitDetailsTypeEnum = "PRIVATE"
)

var mappingCreateVirtualCircuitDetailsType = map[string]CreateVirtualCircuitDetailsTypeEnum{
	"PUBLIC":  CreateVirtualCircuitDetailsTypePublic,
	"PRIVATE": CreateVirtualCircuitDetailsTypePrivate,
}

// GetCreateVirtualCircuitDetailsTypeEnumValues Enumerates the set of values for CreateVirtualCircuitDetailsType
func GetCreateVirtualCircuitDetailsTypeEnumValues() []CreateVirtualCircuitDetailsTypeEnum {
	values := make([]CreateVirtualCircuitDetailsTypeEnum, 0)
	for _, v := range mappingCreateVirtualCircuitDetailsType {
		values = append(values, v)
	}
	return values
}
