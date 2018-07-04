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

// InstanceCredentials The credentials for a particular instance.
type InstanceCredentials struct {

	// The password for the username.
	Password *string `mandatory:"true" json:"password"`

	// The username.
	Username *string `mandatory:"true" json:"username"`
}

func (m InstanceCredentials) String() string {
	return common.PointerString(m)
}
