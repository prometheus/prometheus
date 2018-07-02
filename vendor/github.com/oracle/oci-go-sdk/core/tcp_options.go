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

// TcpOptions Optional object to specify ports for a TCP rule. If you specify TCP as the
// protocol but omit this object, then all ports are allowed.
type TcpOptions struct {

	// An inclusive range of allowed destination ports. Use the same number for the min and max
	// to indicate a single port. Defaults to all ports if not specified.
	DestinationPortRange *PortRange `mandatory:"false" json:"destinationPortRange"`

	// An inclusive range of allowed source ports. Use the same number for the min and max to
	// indicate a single port. Defaults to all ports if not specified.
	SourcePortRange *PortRange `mandatory:"false" json:"sourcePortRange"`
}

func (m TcpOptions) String() string {
	return common.PointerString(m)
}
