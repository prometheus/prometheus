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

// VirtualCircuitBandwidthShape An individual bandwidth level for virtual circuits.
type VirtualCircuitBandwidthShape struct {

	// The name of the bandwidth shape.
	// Example: `10 Gbps`
	Name *string `mandatory:"true" json:"name"`

	// The bandwidth in Mbps.
	// Example: `10000`
	BandwidthInMbps *int `mandatory:"false" json:"bandwidthInMbps"`
}

func (m VirtualCircuitBandwidthShape) String() string {
	return common.PointerString(m)
}
