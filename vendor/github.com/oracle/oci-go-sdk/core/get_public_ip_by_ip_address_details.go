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

// GetPublicIpByIpAddressDetails IP address of the public IP.
type GetPublicIpByIpAddressDetails struct {

	// The public IP address.
	// Example: 129.146.2.1
	IpAddress *string `mandatory:"true" json:"ipAddress"`
}

func (m GetPublicIpByIpAddressDetails) String() string {
	return common.PointerString(m)
}
