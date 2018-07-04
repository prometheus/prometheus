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

// CreateCrossConnectGroupDetails The representation of CreateCrossConnectGroupDetails
type CreateCrossConnectGroupDetails struct {

	// The OCID of the compartment to contain the cross-connect group.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`
}

func (m CreateCrossConnectGroupDetails) String() string {
	return common.PointerString(m)
}
