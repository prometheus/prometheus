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

// PublicIp A *public IP* is a conceptual term that refers to a public IP address and related properties.
// The `publicIp` object is the API representation of a public IP.
// There are two types of public IPs:
// 1. Ephemeral
// 2. Reserved
// For more information and comparison of the two types,
// see Public IP Addresses (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingpublicIPs.htm).
type PublicIp struct {

	// The public IP's Availability Domain. This property is set only for ephemeral public IPs
	// (that is, when the `scope` of the public IP is set to AVAILABILITY_DOMAIN). The value
	// is the Availability Domain of the assigned private IP.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"false" json:"availabilityDomain"`

	// The OCID of the compartment containing the public IP. For an ephemeral public IP, this is
	// the same compartment as the private IP's. For a reserved public IP that is currently assigned,
	// this can be a different compartment than the assigned private IP's.
	CompartmentId *string `mandatory:"false" json:"compartmentId"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable. Avoid
	// entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// The public IP's Oracle ID (OCID).
	Id *string `mandatory:"false" json:"id"`

	// The public IP address of the `publicIp` object.
	// Example: `129.146.2.1`
	IpAddress *string `mandatory:"false" json:"ipAddress"`

	// The public IP's current state.
	LifecycleState PublicIpLifecycleStateEnum `mandatory:"false" json:"lifecycleState,omitempty"`

	// Defines when the public IP is deleted and released back to Oracle's public IP pool.
	// * `EPHEMERAL`: The lifetime is tied to the lifetime of its assigned private IP. The
	// ephemeral public IP is automatically deleted when its private IP is deleted, when
	// the VNIC is terminated, or when the instance is terminated. An ephemeral
	// public IP must always be assigned to a private IP.
	// * `RESERVED`: You control the public IP's lifetime. You can delete a reserved public IP
	// whenever you like. It does not need to be assigned to a private IP at all times.
	// For more information and comparison of the two types,
	// see Public IP Addresses (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingpublicIPs.htm).
	Lifetime PublicIpLifetimeEnum `mandatory:"false" json:"lifetime,omitempty"`

	// The OCID of the private IP that the public IP is currently assigned to, or in the
	// process of being assigned to.
	PrivateIpId *string `mandatory:"false" json:"privateIpId"`

	// Whether the public IP is regional or specific to a particular Availability Domain.
	// * `REGION`: The public IP exists within a region and can be assigned to a private IP
	// in any Availability Domain in the region. Reserved public IPs have `scope` = `REGION`.
	// * `AVAILABILITY_DOMAIN`: The public IP exists within the Availability Domain of the private IP
	// it's assigned to, which is specified by the `availabilityDomain` property of the public IP object.
	// Ephemeral public IPs have `scope` = `AVAILABILITY_DOMAIN`.
	Scope PublicIpScopeEnum `mandatory:"false" json:"scope,omitempty"`

	// The date and time the public IP was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m PublicIp) String() string {
	return common.PointerString(m)
}

// PublicIpLifecycleStateEnum Enum with underlying type: string
type PublicIpLifecycleStateEnum string

// Set of constants representing the allowable values for PublicIpLifecycleState
const (
	PublicIpLifecycleStateProvisioning PublicIpLifecycleStateEnum = "PROVISIONING"
	PublicIpLifecycleStateAvailable    PublicIpLifecycleStateEnum = "AVAILABLE"
	PublicIpLifecycleStateAssigning    PublicIpLifecycleStateEnum = "ASSIGNING"
	PublicIpLifecycleStateAssigned     PublicIpLifecycleStateEnum = "ASSIGNED"
	PublicIpLifecycleStateUnassigning  PublicIpLifecycleStateEnum = "UNASSIGNING"
	PublicIpLifecycleStateUnassigned   PublicIpLifecycleStateEnum = "UNASSIGNED"
	PublicIpLifecycleStateTerminating  PublicIpLifecycleStateEnum = "TERMINATING"
	PublicIpLifecycleStateTerminated   PublicIpLifecycleStateEnum = "TERMINATED"
)

var mappingPublicIpLifecycleState = map[string]PublicIpLifecycleStateEnum{
	"PROVISIONING": PublicIpLifecycleStateProvisioning,
	"AVAILABLE":    PublicIpLifecycleStateAvailable,
	"ASSIGNING":    PublicIpLifecycleStateAssigning,
	"ASSIGNED":     PublicIpLifecycleStateAssigned,
	"UNASSIGNING":  PublicIpLifecycleStateUnassigning,
	"UNASSIGNED":   PublicIpLifecycleStateUnassigned,
	"TERMINATING":  PublicIpLifecycleStateTerminating,
	"TERMINATED":   PublicIpLifecycleStateTerminated,
}

// GetPublicIpLifecycleStateEnumValues Enumerates the set of values for PublicIpLifecycleState
func GetPublicIpLifecycleStateEnumValues() []PublicIpLifecycleStateEnum {
	values := make([]PublicIpLifecycleStateEnum, 0)
	for _, v := range mappingPublicIpLifecycleState {
		values = append(values, v)
	}
	return values
}

// PublicIpLifetimeEnum Enum with underlying type: string
type PublicIpLifetimeEnum string

// Set of constants representing the allowable values for PublicIpLifetime
const (
	PublicIpLifetimeEphemeral PublicIpLifetimeEnum = "EPHEMERAL"
	PublicIpLifetimeReserved  PublicIpLifetimeEnum = "RESERVED"
)

var mappingPublicIpLifetime = map[string]PublicIpLifetimeEnum{
	"EPHEMERAL": PublicIpLifetimeEphemeral,
	"RESERVED":  PublicIpLifetimeReserved,
}

// GetPublicIpLifetimeEnumValues Enumerates the set of values for PublicIpLifetime
func GetPublicIpLifetimeEnumValues() []PublicIpLifetimeEnum {
	values := make([]PublicIpLifetimeEnum, 0)
	for _, v := range mappingPublicIpLifetime {
		values = append(values, v)
	}
	return values
}

// PublicIpScopeEnum Enum with underlying type: string
type PublicIpScopeEnum string

// Set of constants representing the allowable values for PublicIpScope
const (
	PublicIpScopeRegion             PublicIpScopeEnum = "REGION"
	PublicIpScopeAvailabilityDomain PublicIpScopeEnum = "AVAILABILITY_DOMAIN"
)

var mappingPublicIpScope = map[string]PublicIpScopeEnum{
	"REGION":              PublicIpScopeRegion,
	"AVAILABILITY_DOMAIN": PublicIpScopeAvailabilityDomain,
}

// GetPublicIpScopeEnumValues Enumerates the set of values for PublicIpScope
func GetPublicIpScopeEnumValues() []PublicIpScopeEnum {
	values := make([]PublicIpScopeEnum, 0)
	for _, v := range mappingPublicIpScope {
		values = append(values, v)
	}
	return values
}
