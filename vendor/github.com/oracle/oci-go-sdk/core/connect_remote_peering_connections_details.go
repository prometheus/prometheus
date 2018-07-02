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

// ConnectRemotePeeringConnectionsDetails Information about the other remote peering connection (RPC).
type ConnectRemotePeeringConnectionsDetails struct {

	// The OCID of the RPC you want to peer with.
	PeerId *string `mandatory:"true" json:"peerId"`

	// The name of the region that contains the RPC you want to peer with.
	// Example: `us-ashburn-1`
	PeerRegionName *string `mandatory:"true" json:"peerRegionName"`
}

func (m ConnectRemotePeeringConnectionsDetails) String() string {
	return common.PointerString(m)
}
