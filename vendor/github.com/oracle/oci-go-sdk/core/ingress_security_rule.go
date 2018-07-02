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

// IngressSecurityRule A rule for allowing inbound IP packets.
type IngressSecurityRule struct {

	// The transport protocol. Specify either `all` or an IPv4 protocol number as
	// defined in
	// Protocol Numbers (http://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml).
	// Options are supported only for ICMP ("1"), TCP ("6"), and UDP ("17").
	Protocol *string `mandatory:"true" json:"protocol"`

	// The source service cidrBlock or source IP address range in CIDR notation for the ingress rule. This is the
	// range of IP addresses that a packet coming into the instance can come from.
	// Examples: `10.12.0.0/16`
	//           `oci-phx-objectstorage`
	Source *string `mandatory:"true" json:"source"`

	// Optional and valid only for ICMP. Use to specify a particular ICMP type and code
	// as defined in
	// ICMP Parameters (http://www.iana.org/assignments/icmp-parameters/icmp-parameters.xhtml).
	// If you specify ICMP as the protocol but omit this object, then all ICMP types and
	// codes are allowed. If you do provide this object, the type is required and the code is optional.
	// To enable MTU negotiation for ingress internet traffic, make sure to allow type 3 ("Destination
	// Unreachable") code 4 ("Fragmentation Needed and Don't Fragment was Set"). If you need to specify
	// multiple codes for a single type, create a separate security list rule for each.
	IcmpOptions *IcmpOptions `mandatory:"false" json:"icmpOptions"`

	// A stateless rule allows traffic in one direction. Remember to add a corresponding
	// stateless rule in the other direction if you need to support bidirectional traffic. For
	// example, if ingress traffic allows TCP destination port 80, there should be an egress
	// rule to allow TCP source port 80. Defaults to false, which means the rule is stateful
	// and a corresponding rule is not necessary for bidirectional traffic.
	IsStateless *bool `mandatory:"false" json:"isStateless"`

	// Type of source for IngressSecurityRule. SERVICE_CIDR_BLOCK should be used if source is a service cidrBlock.
	// CIDR_BLOCK should be used if source is IP address range in CIDR notation. It defaults to CIDR_BLOCK, if
	// not specified.
	SourceType IngressSecurityRuleSourceTypeEnum `mandatory:"false" json:"sourceType,omitempty"`

	// Optional and valid only for TCP. Use to specify particular destination ports for TCP rules.
	// If you specify TCP as the protocol but omit this object, then all destination ports are allowed.
	TcpOptions *TcpOptions `mandatory:"false" json:"tcpOptions"`

	// Optional and valid only for UDP. Use to specify particular destination ports for UDP rules.
	// If you specify UDP as the protocol but omit this object, then all destination ports are allowed.
	UdpOptions *UdpOptions `mandatory:"false" json:"udpOptions"`
}

func (m IngressSecurityRule) String() string {
	return common.PointerString(m)
}

// IngressSecurityRuleSourceTypeEnum Enum with underlying type: string
type IngressSecurityRuleSourceTypeEnum string

// Set of constants representing the allowable values for IngressSecurityRuleSourceType
const (
	IngressSecurityRuleSourceTypeCidrBlock        IngressSecurityRuleSourceTypeEnum = "CIDR_BLOCK"
	IngressSecurityRuleSourceTypeServiceCidrBlock IngressSecurityRuleSourceTypeEnum = "SERVICE_CIDR_BLOCK"
)

var mappingIngressSecurityRuleSourceType = map[string]IngressSecurityRuleSourceTypeEnum{
	"CIDR_BLOCK":         IngressSecurityRuleSourceTypeCidrBlock,
	"SERVICE_CIDR_BLOCK": IngressSecurityRuleSourceTypeServiceCidrBlock,
}

// GetIngressSecurityRuleSourceTypeEnumValues Enumerates the set of values for IngressSecurityRuleSourceType
func GetIngressSecurityRuleSourceTypeEnumValues() []IngressSecurityRuleSourceTypeEnum {
	values := make([]IngressSecurityRuleSourceTypeEnum, 0)
	for _, v := range mappingIngressSecurityRuleSourceType {
		values = append(values, v)
	}
	return values
}
