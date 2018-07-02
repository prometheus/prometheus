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

// PortRange The representation of PortRange
type PortRange struct {

	// The maximum port number. Must not be lower than the minimum port number. To specify
	// a single port number, set both the min and max to the same value.
	Max *int `mandatory:"true" json:"max"`

	// The minimum port number. Must not be greater than the maximum port number.
	Min *int `mandatory:"true" json:"min"`
}

func (m PortRange) String() string {
	return common.PointerString(m)
}
