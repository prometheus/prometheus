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

// CreateVirtualCircuitPublicPrefixDetails The representation of CreateVirtualCircuitPublicPrefixDetails
type CreateVirtualCircuitPublicPrefixDetails struct {

	// An individual public IP prefix (CIDR) to add to the public virtual circuit.
	// Must be /24 or less specific.
	CidrBlock *string `mandatory:"true" json:"cidrBlock"`
}

func (m CreateVirtualCircuitPublicPrefixDetails) String() string {
	return common.PointerString(m)
}
