// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

// Core Services API
//
// APIs for Networking Service, Compute Service, and Block Volume Service.
//

package core

import (
	"encoding/json"
	"github.com/oracle/oci-go-sdk/common"
)

// DhcpSearchDomainOption DHCP option for specifying a search domain name for DNS queries. For more information, see
// DNS in Your Virtual Cloud Network (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/dns.htm).
type DhcpSearchDomainOption struct {

	// A single search domain name according to RFC 952 (https://tools.ietf.org/html/rfc952)
	// and RFC 1123 (https://tools.ietf.org/html/rfc1123). During a DNS query,
	// the OS will append this search domain name to the value being queried.
	// If you set DhcpDnsOption to `VcnLocalPlusInternet`,
	// and you assign a DNS label to the VCN during creation, the search domain name in the
	// VCN's default set of DHCP options is automatically set to the VCN domain
	// (for example, `vcn1.oraclevcn.com`).
	// If you don't want to use a search domain name, omit this option from the
	// set of DHCP options. Do not include this option with an empty list
	// of search domain names, or with an empty string as the value for any search
	// domain name.
	SearchDomainNames []string `mandatory:"true" json:"searchDomainNames"`
}

func (m DhcpSearchDomainOption) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m DhcpSearchDomainOption) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeDhcpSearchDomainOption DhcpSearchDomainOption
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeDhcpSearchDomainOption
	}{
		"SearchDomain",
		(MarshalTypeDhcpSearchDomainOption)(m),
	}

	return json.Marshal(&s)
}
