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

// RemotePeeringConnection A remote peering connection (RPC) is an object on a DRG that lets the VCN that is attached
// to the DRG peer with a VCN in a different region. *Peering* means that the two VCNs can
// communicate using private IP addresses, but without the traffic traversing the internet or
// routing through your on-premises network. For more information, see
// VCN Peering (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/VCNpeering.htm).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type RemotePeeringConnection struct {

	// The OCID of the compartment that contains the RPC.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"true" json:"displayName"`

	// The OCID of the DRG that this RPC belongs to.
	DrgId *string `mandatory:"true" json:"drgId"`

	// The OCID of the RPC.
	Id *string `mandatory:"true" json:"id"`

	// Whether the VCN at the other end of the peering is in a different tenancy.
	// Example: `false`
	IsCrossTenancyPeering *bool `mandatory:"true" json:"isCrossTenancyPeering"`

	// The RPC's current lifecycle state.
	LifecycleState RemotePeeringConnectionLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// Whether the RPC is peered with another RPC. `NEW` means the RPC has not yet been
	// peered. `PENDING` means the peering is being established. `REVOKED` means the
	// RPC at the other end of the peering has been deleted.
	PeeringStatus RemotePeeringConnectionPeeringStatusEnum `mandatory:"true" json:"peeringStatus"`

	// The date and time the RPC was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`

	// If this RPC is peered, this value is the OCID of the other RPC.
	PeerId *string `mandatory:"false" json:"peerId"`

	// If this RPC is peered, this value is the region that contains the other RPC.
	// Example: `us-ashburn-1`
	PeerRegionName *string `mandatory:"false" json:"peerRegionName"`

	// If this RPC is peered, this value is the OCID of the other RPC's tenancy.
	PeerTenancyId *string `mandatory:"false" json:"peerTenancyId"`
}

func (m RemotePeeringConnection) String() string {
	return common.PointerString(m)
}

// RemotePeeringConnectionLifecycleStateEnum Enum with underlying type: string
type RemotePeeringConnectionLifecycleStateEnum string

// Set of constants representing the allowable values for RemotePeeringConnectionLifecycleState
const (
	RemotePeeringConnectionLifecycleStateAvailable    RemotePeeringConnectionLifecycleStateEnum = "AVAILABLE"
	RemotePeeringConnectionLifecycleStateProvisioning RemotePeeringConnectionLifecycleStateEnum = "PROVISIONING"
	RemotePeeringConnectionLifecycleStateTerminating  RemotePeeringConnectionLifecycleStateEnum = "TERMINATING"
	RemotePeeringConnectionLifecycleStateTerminated   RemotePeeringConnectionLifecycleStateEnum = "TERMINATED"
)

var mappingRemotePeeringConnectionLifecycleState = map[string]RemotePeeringConnectionLifecycleStateEnum{
	"AVAILABLE":    RemotePeeringConnectionLifecycleStateAvailable,
	"PROVISIONING": RemotePeeringConnectionLifecycleStateProvisioning,
	"TERMINATING":  RemotePeeringConnectionLifecycleStateTerminating,
	"TERMINATED":   RemotePeeringConnectionLifecycleStateTerminated,
}

// GetRemotePeeringConnectionLifecycleStateEnumValues Enumerates the set of values for RemotePeeringConnectionLifecycleState
func GetRemotePeeringConnectionLifecycleStateEnumValues() []RemotePeeringConnectionLifecycleStateEnum {
	values := make([]RemotePeeringConnectionLifecycleStateEnum, 0)
	for _, v := range mappingRemotePeeringConnectionLifecycleState {
		values = append(values, v)
	}
	return values
}

// RemotePeeringConnectionPeeringStatusEnum Enum with underlying type: string
type RemotePeeringConnectionPeeringStatusEnum string

// Set of constants representing the allowable values for RemotePeeringConnectionPeeringStatus
const (
	RemotePeeringConnectionPeeringStatusInvalid RemotePeeringConnectionPeeringStatusEnum = "INVALID"
	RemotePeeringConnectionPeeringStatusNew     RemotePeeringConnectionPeeringStatusEnum = "NEW"
	RemotePeeringConnectionPeeringStatusPending RemotePeeringConnectionPeeringStatusEnum = "PENDING"
	RemotePeeringConnectionPeeringStatusPeered  RemotePeeringConnectionPeeringStatusEnum = "PEERED"
	RemotePeeringConnectionPeeringStatusRevoked RemotePeeringConnectionPeeringStatusEnum = "REVOKED"
)

var mappingRemotePeeringConnectionPeeringStatus = map[string]RemotePeeringConnectionPeeringStatusEnum{
	"INVALID": RemotePeeringConnectionPeeringStatusInvalid,
	"NEW":     RemotePeeringConnectionPeeringStatusNew,
	"PENDING": RemotePeeringConnectionPeeringStatusPending,
	"PEERED":  RemotePeeringConnectionPeeringStatusPeered,
	"REVOKED": RemotePeeringConnectionPeeringStatusRevoked,
}

// GetRemotePeeringConnectionPeeringStatusEnumValues Enumerates the set of values for RemotePeeringConnectionPeeringStatus
func GetRemotePeeringConnectionPeeringStatusEnumValues() []RemotePeeringConnectionPeeringStatusEnum {
	values := make([]RemotePeeringConnectionPeeringStatusEnum, 0)
	for _, v := range mappingRemotePeeringConnectionPeeringStatus {
		values = append(values, v)
	}
	return values
}
