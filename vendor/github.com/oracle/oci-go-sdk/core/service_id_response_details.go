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

// ServiceIdResponseDetails The representation of ServiceIdResponseDetails
type ServiceIdResponseDetails struct {

	// The OCID (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm) of the service.
	ServiceId *string `mandatory:"true" json:"serviceId"`

	// The name of the service.
	ServiceName *string `mandatory:"true" json:"serviceName"`
}

func (m ServiceIdResponseDetails) String() string {
	return common.PointerString(m)
}
