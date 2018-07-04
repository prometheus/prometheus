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

// DeleteVirtualCircuitPublicPrefixDetails The representation of DeleteVirtualCircuitPublicPrefixDetails
type DeleteVirtualCircuitPublicPrefixDetails struct {

	// An individual public IP prefix (CIDR) to remove from the public virtual circuit.
	CidrBlock *string `mandatory:"true" json:"cidrBlock"`
}

func (m DeleteVirtualCircuitPublicPrefixDetails) String() string {
	return common.PointerString(m)
}
