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

// EgressSecurityRule A rule for allowing outbound IP packets.
type EgressSecurityRule struct {

	// The destination service cidrBlock or destination IP address range in CIDR notation for the egress rule.
	// This is the range of IP addresses that a packet originating from the instance can go to.
	Destination *string `mandatory:"true" json:"destination"`

	// The transport protocol. Specify either `all` or an IPv4 protocol number as
	// defined in
	// Protocol Numbers (http://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml).
	// Options are supported only for ICMP ("1"), TCP ("6"), and UDP ("17").
	Protocol *string `mandatory:"true" json:"protocol"`

	// Type of destination for EgressSecurityRule. SERVICE_CIDR_BLOCK should be used if destination is a service
	// cidrBlock. CIDR_BLOCK should be used if destination is IP address range in CIDR notation.
	// It defaults to CIDR_BLOCK, if not specified.
	DestinationType EgressSecurityRuleDestinationTypeEnum `mandatory:"false" json:"destinationType,omitempty"`

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
	// example, if egress traffic allows TCP destination port 80, there should be an ingress
	// rule to allow TCP source port 80. Defaults to false, which means the rule is stateful
	// and a corresponding rule is not necessary for bidirectional traffic.
	IsStateless *bool `mandatory:"false" json:"isStateless"`

	// Optional and valid only for TCP. Use to specify particular destination ports for TCP rules.
	// If you specify TCP as the protocol but omit this object, then all destination ports are allowed.
	TcpOptions *TcpOptions `mandatory:"false" json:"tcpOptions"`

	// Optional and valid only for UDP. Use to specify particular destination ports for UDP rules.
	// If you specify UDP as the protocol but omit this object, then all destination ports are allowed.
	UdpOptions *UdpOptions `mandatory:"false" json:"udpOptions"`
}

func (m EgressSecurityRule) String() string {
	return common.PointerString(m)
}

// EgressSecurityRuleDestinationTypeEnum Enum with underlying type: string
type EgressSecurityRuleDestinationTypeEnum string

// Set of constants representing the allowable values for EgressSecurityRuleDestinationType
const (
	EgressSecurityRuleDestinationTypeCidrBlock        EgressSecurityRuleDestinationTypeEnum = "CIDR_BLOCK"
	EgressSecurityRuleDestinationTypeServiceCidrBlock EgressSecurityRuleDestinationTypeEnum = "SERVICE_CIDR_BLOCK"
)

var mappingEgressSecurityRuleDestinationType = map[string]EgressSecurityRuleDestinationTypeEnum{
	"CIDR_BLOCK":         EgressSecurityRuleDestinationTypeCidrBlock,
	"SERVICE_CIDR_BLOCK": EgressSecurityRuleDestinationTypeServiceCidrBlock,
}

// GetEgressSecurityRuleDestinationTypeEnumValues Enumerates the set of values for EgressSecurityRuleDestinationType
func GetEgressSecurityRuleDestinationTypeEnumValues() []EgressSecurityRuleDestinationTypeEnum {
	values := make([]EgressSecurityRuleDestinationTypeEnum, 0)
	for _, v := range mappingEgressSecurityRuleDestinationType {
		values = append(values, v)
	}
	return values
}
