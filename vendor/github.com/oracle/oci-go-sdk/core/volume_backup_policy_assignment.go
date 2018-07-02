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

// VolumeBackupPolicyAssignment Specifies that a particular volume backup policy is assigned to an asset such as a volume.
type VolumeBackupPolicyAssignment struct {

	// The OCID of the asset (e.g. a volume) to which the policy has been assigned.
	AssetId *string `mandatory:"true" json:"assetId"`

	// The OCID of the volume backup policy assignment.
	Id *string `mandatory:"true" json:"id"`

	// The OCID of the volume backup policy that has been assigned to an asset.
	PolicyId *string `mandatory:"true" json:"policyId"`

	// The date and time the volume backup policy assignment was created. Format defined by RFC3339.
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`
}

func (m VolumeBackupPolicyAssignment) String() string {
	return common.PointerString(m)
}
