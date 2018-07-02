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

// VirtualCircuit For use with Oracle Cloud Infrastructure FastConnect.
// A virtual circuit is an isolated network path that runs over one or more physical
// network connections to provide a single, logical connection between the edge router
// on the customer's existing network and Oracle Cloud Infrastructure. *Private*
// virtual circuits support private peering, and *public* virtual circuits support
// public peering. For more information, see FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
// Each virtual circuit is made up of information shared between a customer, Oracle,
// and a provider (if the customer is using FastConnect via a provider). Who fills in
// a given property of a virtual circuit depends on whether the BGP session related to
// that virtual circuit goes from the customer's edge router to Oracle, or from the provider's
// edge router to Oracle. Also, in the case where the customer is using a provider, values
// for some of the properties may not be present immediately, but may get filled in as the
// provider and Oracle each do their part to provision the virtual circuit.
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type VirtualCircuit struct {

	// The provisioned data rate of the connection.
	BandwidthShapeName *string `mandatory:"false" json:"bandwidthShapeName"`

	// BGP management option.
	BgpManagement VirtualCircuitBgpManagementEnum `mandatory:"false" json:"bgpManagement,omitempty"`

	// The state of the BGP session associated with the virtual circuit.
	BgpSessionState VirtualCircuitBgpSessionStateEnum `mandatory:"false" json:"bgpSessionState,omitempty"`

	// The OCID of the compartment containing the virtual circuit.
	CompartmentId *string `mandatory:"false" json:"compartmentId"`

	// An array of mappings, each containing properties for a
	// cross-connect or cross-connect group that is associated with this
	// virtual circuit.
	CrossConnectMappings []CrossConnectMapping `mandatory:"false" json:"crossConnectMappings"`

	// The BGP ASN of the network at the other end of the BGP
	// session from Oracle. If the session is between the customer's
	// edge router and Oracle, the value is the customer's ASN. If the BGP
	// session is between the provider's edge router and Oracle, the value
	// is the provider's ASN.
	CustomerBgpAsn *int `mandatory:"false" json:"customerBgpAsn"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// The OCID of the customer's Drg
	// that this virtual circuit uses. Applicable only to private virtual circuits.
	GatewayId *string `mandatory:"false" json:"gatewayId"`

	// The virtual circuit's Oracle ID (OCID).
	Id *string `mandatory:"false" json:"id"`

	// The virtual circuit's current state. For information about
	// the different states, see
	// FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
	LifecycleState VirtualCircuitLifecycleStateEnum `mandatory:"false" json:"lifecycleState,omitempty"`

	// The Oracle BGP ASN.
	OracleBgpAsn *int `mandatory:"false" json:"oracleBgpAsn"`

	// Deprecated. Instead use `providerServiceId`.
	ProviderName *string `mandatory:"false" json:"providerName"`

	// The OCID of the service offered by the provider (if the customer is connecting via a provider).
	ProviderServiceId *string `mandatory:"false" json:"providerServiceId"`

	// Deprecated. Instead use `providerServiceId`.
	ProviderServiceName *string `mandatory:"false" json:"providerServiceName"`

	// The provider's state in relation to this virtual circuit (if the
	// customer is connecting via a provider). ACTIVE means
	// the provider has provisioned the virtual circuit from their end.
	// INACTIVE means the provider has not yet provisioned the virtual
	// circuit, or has de-provisioned it.
	ProviderState VirtualCircuitProviderStateEnum `mandatory:"false" json:"providerState,omitempty"`

	// For a public virtual circuit. The public IP prefixes (CIDRs) the customer wants to
	// advertise across the connection. Each prefix must be /24 or less specific.
	PublicPrefixes []string `mandatory:"false" json:"publicPrefixes"`

	// Provider-supplied reference information about this virtual circuit
	// (if the customer is connecting via a provider).
	ReferenceComment *string `mandatory:"false" json:"referenceComment"`

	// The Oracle Cloud Infrastructure region where this virtual
	// circuit is located.
	Region *string `mandatory:"false" json:"region"`

	// Provider service type.
	ServiceType VirtualCircuitServiceTypeEnum `mandatory:"false" json:"serviceType,omitempty"`

	// The date and time the virtual circuit was created,
	// in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`

	// Whether the virtual circuit supports private or public peering. For more information,
	// see FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
	Type VirtualCircuitTypeEnum `mandatory:"false" json:"type,omitempty"`
}

func (m VirtualCircuit) String() string {
	return common.PointerString(m)
}

// VirtualCircuitBgpManagementEnum Enum with underlying type: string
type VirtualCircuitBgpManagementEnum string

// Set of constants representing the allowable values for VirtualCircuitBgpManagement
const (
	VirtualCircuitBgpManagementCustomerManaged VirtualCircuitBgpManagementEnum = "CUSTOMER_MANAGED"
	VirtualCircuitBgpManagementProviderManaged VirtualCircuitBgpManagementEnum = "PROVIDER_MANAGED"
	VirtualCircuitBgpManagementOracleManaged   VirtualCircuitBgpManagementEnum = "ORACLE_MANAGED"
)

var mappingVirtualCircuitBgpManagement = map[string]VirtualCircuitBgpManagementEnum{
	"CUSTOMER_MANAGED": VirtualCircuitBgpManagementCustomerManaged,
	"PROVIDER_MANAGED": VirtualCircuitBgpManagementProviderManaged,
	"ORACLE_MANAGED":   VirtualCircuitBgpManagementOracleManaged,
}

// GetVirtualCircuitBgpManagementEnumValues Enumerates the set of values for VirtualCircuitBgpManagement
func GetVirtualCircuitBgpManagementEnumValues() []VirtualCircuitBgpManagementEnum {
	values := make([]VirtualCircuitBgpManagementEnum, 0)
	for _, v := range mappingVirtualCircuitBgpManagement {
		values = append(values, v)
	}
	return values
}

// VirtualCircuitBgpSessionStateEnum Enum with underlying type: string
type VirtualCircuitBgpSessionStateEnum string

// Set of constants representing the allowable values for VirtualCircuitBgpSessionState
const (
	VirtualCircuitBgpSessionStateUp   VirtualCircuitBgpSessionStateEnum = "UP"
	VirtualCircuitBgpSessionStateDown VirtualCircuitBgpSessionStateEnum = "DOWN"
)

var mappingVirtualCircuitBgpSessionState = map[string]VirtualCircuitBgpSessionStateEnum{
	"UP":   VirtualCircuitBgpSessionStateUp,
	"DOWN": VirtualCircuitBgpSessionStateDown,
}

// GetVirtualCircuitBgpSessionStateEnumValues Enumerates the set of values for VirtualCircuitBgpSessionState
func GetVirtualCircuitBgpSessionStateEnumValues() []VirtualCircuitBgpSessionStateEnum {
	values := make([]VirtualCircuitBgpSessionStateEnum, 0)
	for _, v := range mappingVirtualCircuitBgpSessionState {
		values = append(values, v)
	}
	return values
}

// VirtualCircuitLifecycleStateEnum Enum with underlying type: string
type VirtualCircuitLifecycleStateEnum string

// Set of constants representing the allowable values for VirtualCircuitLifecycleState
const (
	VirtualCircuitLifecycleStatePendingProvider VirtualCircuitLifecycleStateEnum = "PENDING_PROVIDER"
	VirtualCircuitLifecycleStateVerifying       VirtualCircuitLifecycleStateEnum = "VERIFYING"
	VirtualCircuitLifecycleStateProvisioning    VirtualCircuitLifecycleStateEnum = "PROVISIONING"
	VirtualCircuitLifecycleStateProvisioned     VirtualCircuitLifecycleStateEnum = "PROVISIONED"
	VirtualCircuitLifecycleStateFailed          VirtualCircuitLifecycleStateEnum = "FAILED"
	VirtualCircuitLifecycleStateInactive        VirtualCircuitLifecycleStateEnum = "INACTIVE"
	VirtualCircuitLifecycleStateTerminating     VirtualCircuitLifecycleStateEnum = "TERMINATING"
	VirtualCircuitLifecycleStateTerminated      VirtualCircuitLifecycleStateEnum = "TERMINATED"
)

var mappingVirtualCircuitLifecycleState = map[string]VirtualCircuitLifecycleStateEnum{
	"PENDING_PROVIDER": VirtualCircuitLifecycleStatePendingProvider,
	"VERIFYING":        VirtualCircuitLifecycleStateVerifying,
	"PROVISIONING":     VirtualCircuitLifecycleStateProvisioning,
	"PROVISIONED":      VirtualCircuitLifecycleStateProvisioned,
	"FAILED":           VirtualCircuitLifecycleStateFailed,
	"INACTIVE":         VirtualCircuitLifecycleStateInactive,
	"TERMINATING":      VirtualCircuitLifecycleStateTerminating,
	"TERMINATED":       VirtualCircuitLifecycleStateTerminated,
}

// GetVirtualCircuitLifecycleStateEnumValues Enumerates the set of values for VirtualCircuitLifecycleState
func GetVirtualCircuitLifecycleStateEnumValues() []VirtualCircuitLifecycleStateEnum {
	values := make([]VirtualCircuitLifecycleStateEnum, 0)
	for _, v := range mappingVirtualCircuitLifecycleState {
		values = append(values, v)
	}
	return values
}

// VirtualCircuitProviderStateEnum Enum with underlying type: string
type VirtualCircuitProviderStateEnum string

// Set of constants representing the allowable values for VirtualCircuitProviderState
const (
	VirtualCircuitProviderStateActive   VirtualCircuitProviderStateEnum = "ACTIVE"
	VirtualCircuitProviderStateInactive VirtualCircuitProviderStateEnum = "INACTIVE"
)

var mappingVirtualCircuitProviderState = map[string]VirtualCircuitProviderStateEnum{
	"ACTIVE":   VirtualCircuitProviderStateActive,
	"INACTIVE": VirtualCircuitProviderStateInactive,
}

// GetVirtualCircuitProviderStateEnumValues Enumerates the set of values for VirtualCircuitProviderState
func GetVirtualCircuitProviderStateEnumValues() []VirtualCircuitProviderStateEnum {
	values := make([]VirtualCircuitProviderStateEnum, 0)
	for _, v := range mappingVirtualCircuitProviderState {
		values = append(values, v)
	}
	return values
}

// VirtualCircuitServiceTypeEnum Enum with underlying type: string
type VirtualCircuitServiceTypeEnum string

// Set of constants representing the allowable values for VirtualCircuitServiceType
const (
	VirtualCircuitServiceTypeColocated VirtualCircuitServiceTypeEnum = "COLOCATED"
	VirtualCircuitServiceTypeLayer2    VirtualCircuitServiceTypeEnum = "LAYER2"
	VirtualCircuitServiceTypeLayer3    VirtualCircuitServiceTypeEnum = "LAYER3"
)

var mappingVirtualCircuitServiceType = map[string]VirtualCircuitServiceTypeEnum{
	"COLOCATED": VirtualCircuitServiceTypeColocated,
	"LAYER2":    VirtualCircuitServiceTypeLayer2,
	"LAYER3":    VirtualCircuitServiceTypeLayer3,
}

// GetVirtualCircuitServiceTypeEnumValues Enumerates the set of values for VirtualCircuitServiceType
func GetVirtualCircuitServiceTypeEnumValues() []VirtualCircuitServiceTypeEnum {
	values := make([]VirtualCircuitServiceTypeEnum, 0)
	for _, v := range mappingVirtualCircuitServiceType {
		values = append(values, v)
	}
	return values
}

// VirtualCircuitTypeEnum Enum with underlying type: string
type VirtualCircuitTypeEnum string

// Set of constants representing the allowable values for VirtualCircuitType
const (
	VirtualCircuitTypePublic  VirtualCircuitTypeEnum = "PUBLIC"
	VirtualCircuitTypePrivate VirtualCircuitTypeEnum = "PRIVATE"
)

var mappingVirtualCircuitType = map[string]VirtualCircuitTypeEnum{
	"PUBLIC":  VirtualCircuitTypePublic,
	"PRIVATE": VirtualCircuitTypePrivate,
}

// GetVirtualCircuitTypeEnumValues Enumerates the set of values for VirtualCircuitType
func GetVirtualCircuitTypeEnumValues() []VirtualCircuitTypeEnum {
	values := make([]VirtualCircuitTypeEnum, 0)
	for _, v := range mappingVirtualCircuitType {
		values = append(values, v)
	}
	return values
}
