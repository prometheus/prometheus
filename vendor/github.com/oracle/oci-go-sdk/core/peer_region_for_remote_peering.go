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

// PeerRegionForRemotePeering Details about a region that supports remote VCN peering. For more information, see VCN Peering (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/VCNpeering.htm).
type PeerRegionForRemotePeering struct {

	// The region's name.
	// Example: `us-phoenix-1`
	Name *string `mandatory:"true" json:"name"`
}

func (m PeerRegionForRemotePeering) String() string {
	return common.PointerString(m)
}
