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

// Service Information about a service that is accessible through a service gateway.
type Service struct {

	// A string that represents the public endpoints for the service. When you set up a route rule
	// to route traffic to the service gateway, use this value as the destination CIDR block for
	// the rule. See RouteTable.
	CidrBlock *string `mandatory:"true" json:"cidrBlock"`

	// Description of the service.
	Description *string `mandatory:"true" json:"description"`

	// The service's OCID (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
	Id *string `mandatory:"true" json:"id"`

	// Name of the service.
	Name *string `mandatory:"true" json:"name"`
}

func (m Service) String() string {
	return common.PointerString(m)
}
