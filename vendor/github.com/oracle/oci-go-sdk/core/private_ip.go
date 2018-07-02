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

// PrivateIp A *private IP* is a conceptual term that refers to a private IP address and related properties.
// The `privateIp` object is the API representation of a private IP.
// Each instance has a *primary private IP* that is automatically created and
// assigned to the primary VNIC during instance launch. If you add a secondary
// VNIC to the instance, it also automatically gets a primary private IP. You
// can't remove a primary private IP from its VNIC. The primary private IP is
// automatically deleted when the VNIC is terminated.
// You can add *secondary private IPs* to a VNIC after it's created. For more
// information, see the `privateIp` operations and also
// IP Addresses (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingIPaddresses.htm).
// **Note:** Only
// ListPrivateIps and
// GetPrivateIp work with
// *primary* private IPs. To create and update primary private IPs, you instead
// work with instance and VNIC operations. For example, a primary private IP's
// properties come from the values you specify in
// CreateVnicDetails when calling either
// LaunchInstance or
// AttachVnic. To update the hostname
// for a primary private IP, you use UpdateVnic.
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type PrivateIp struct {

	// The private IP's Availability Domain.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"false" json:"availabilityDomain"`

	// The OCID of the compartment containing the private IP.
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

	// The hostname for the private IP. Used for DNS. The value is the hostname
	// portion of the private IP's fully qualified domain name (FQDN)
	// (for example, `bminstance-1` in FQDN `bminstance-1.subnet123.vcn1.oraclevcn.com`).
	// Must be unique across all VNICs in the subnet and comply with
	// RFC 952 (https://tools.ietf.org/html/rfc952) and
	// RFC 1123 (https://tools.ietf.org/html/rfc1123).
	// For more information, see
	// DNS in Your Virtual Cloud Network (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/dns.htm).
	// Example: `bminstance-1`
	HostnameLabel *string `mandatory:"false" json:"hostnameLabel"`

	// The private IP's Oracle ID (OCID).
	Id *string `mandatory:"false" json:"id"`

	// The private IP address of the `privateIp` object. The address is within the CIDR
	// of the VNIC's subnet.
	// Example: `10.0.3.3`
	IpAddress *string `mandatory:"false" json:"ipAddress"`

	// Whether this private IP is the primary one on the VNIC. Primary private IPs
	// are unassigned and deleted automatically when the VNIC is terminated.
	// Example: `true`
	IsPrimary *bool `mandatory:"false" json:"isPrimary"`

	// The OCID of the subnet the VNIC is in.
	SubnetId *string `mandatory:"false" json:"subnetId"`

	// The date and time the private IP was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`

	// The OCID of the VNIC the private IP is assigned to. The VNIC and private IP
	// must be in the same subnet.
	VnicId *string `mandatory:"false" json:"vnicId"`
}

func (m PrivateIp) String() string {
	return common.PointerString(m)
}
