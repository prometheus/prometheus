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

// BulkAddVirtualCircuitPublicPrefixesDetails The representation of BulkAddVirtualCircuitPublicPrefixesDetails
type BulkAddVirtualCircuitPublicPrefixesDetails struct {

	// The public IP prefixes (CIDRs) to add to the public virtual circuit.
	PublicPrefixes []CreateVirtualCircuitPublicPrefixDetails `mandatory:"true" json:"publicPrefixes"`
}

func (m BulkAddVirtualCircuitPublicPrefixesDetails) String() string {
	return common.PointerString(m)
}
