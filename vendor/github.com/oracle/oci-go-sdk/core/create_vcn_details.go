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

// CreateVcnDetails The representation of CreateVcnDetails
type CreateVcnDetails struct {

	// The CIDR IP address block of the VCN.
	// Example: `172.16.0.0/16`
	CidrBlock *string `mandatory:"true" json:"cidrBlock"`

	// The OCID of the compartment to contain the VCN.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable. Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// A DNS label for the VCN, used in conjunction with the VNIC's hostname and
	// subnet's DNS label to form a fully qualified domain name (FQDN) for each VNIC
	// within this subnet (for example, `bminstance-1.subnet123.vcn1.oraclevcn.com`).
	// Not required to be unique, but it's a best practice to set unique DNS labels
	// for VCNs in your tenancy. Must be an alphanumeric string that begins with a letter.
	// The value cannot be changed.
	// You must set this value if you want instances to be able to use hostnames to
	// resolve other instances in the VCN. Otherwise the Internet and VCN Resolver
	// will not work.
	// For more information, see
	// DNS in Your Virtual Cloud Network (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/dns.htm).
	// Example: `vcn1`
	DnsLabel *string `mandatory:"false" json:"dnsLabel"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`
}

func (m CreateVcnDetails) String() string {
	return common.PointerString(m)
}
