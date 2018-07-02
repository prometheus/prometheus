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

// TunnelConfig Specific connection details for an IPSec tunnel.
type TunnelConfig struct {

	// The IP address of Oracle's VPN headend.
	// Example: `129.146.17.50`
	IpAddress *string `mandatory:"true" json:"ipAddress"`

	// The shared secret of the IPSec tunnel.
	// Example: `vFG2IF6TWq4UToUiLSRDoJEUs6j1c.p8G.dVQxiMfMO0yXMLi.lZTbYIWhGu4V8o`
	SharedSecret *string `mandatory:"true" json:"sharedSecret"`

	// The date and time the IPSec connection was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"false" json:"timeCreated"`
}

func (m TunnelConfig) String() string {
	return common.PointerString(m)
}
