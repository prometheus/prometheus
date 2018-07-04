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

// CrossConnectLocation An individual FastConnect location.
type CrossConnectLocation struct {

	// A description of the location.
	Description *string `mandatory:"true" json:"description"`

	// The name of the location.
	// Example: `CyrusOne, Chandler, AZ`
	Name *string `mandatory:"true" json:"name"`
}

func (m CrossConnectLocation) String() string {
	return common.PointerString(m)
}
