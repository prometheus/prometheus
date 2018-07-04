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

// DhcpOption A single DHCP option according to RFC 1533 (https://tools.ietf.org/html/rfc1533).
// The two options available to use are DhcpDnsOption
// and DhcpSearchDomainOption. For more
// information, see DNS in Your Virtual Cloud Network (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/dns.htm)
// and DHCP Options (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingDHCP.htm).
type DhcpOption interface {
}

type dhcpoption struct {
	JsonData []byte
	Type     string `json:"type"`
}

// UnmarshalJSON unmarshals json
func (m *dhcpoption) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalerdhcpoption dhcpoption
	s := struct {
		Model Unmarshalerdhcpoption
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.Type = s.Model.Type

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *dhcpoption) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {
	var err error
	switch m.Type {
	case "DomainNameServer":
		mm := DhcpDnsOption{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "SearchDomain":
		mm := DhcpSearchDomainOption{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		return m, nil
	}
}

func (m dhcpoption) String() string {
	return common.PointerString(m)
}
