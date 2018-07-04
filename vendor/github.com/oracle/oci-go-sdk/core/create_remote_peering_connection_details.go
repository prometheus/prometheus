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

// CreateRemotePeeringConnectionDetails The representation of CreateRemotePeeringConnectionDetails
type CreateRemotePeeringConnectionDetails struct {

	// The OCID of the compartment to contain the RPC.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The OCID of the DRG the RPC belongs to.
	DrgId *string `mandatory:"true" json:"drgId"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`
}

func (m CreateRemotePeeringConnectionDetails) String() string {
	return common.PointerString(m)
}
