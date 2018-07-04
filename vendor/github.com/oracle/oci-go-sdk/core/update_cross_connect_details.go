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

// UpdateCrossConnectDetails Update a CrossConnect
type UpdateCrossConnectDetails struct {

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Set to true to activate the cross-connect. You activate it after the physical cabling
	// is complete, and you've confirmed the cross-connect's light levels are good and your side
	// of the interface is up. Activation indicates to Oracle that the physical connection is ready.
	// Example: `true`
	IsActive *bool `mandatory:"false" json:"isActive"`
}

func (m UpdateCrossConnectDetails) String() string {
	return common.PointerString(m)
}
