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

// ConnectLocalPeeringGatewaysDetails Information about the other local peering gateway (LPG).
type ConnectLocalPeeringGatewaysDetails struct {

	// The OCID of the LPG you want to peer with.
	PeerId *string `mandatory:"true" json:"peerId"`
}

func (m ConnectLocalPeeringGatewaysDetails) String() string {
	return common.PointerString(m)
}
