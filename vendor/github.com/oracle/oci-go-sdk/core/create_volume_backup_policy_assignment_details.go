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

// CreateVolumeBackupPolicyAssignmentDetails The representation of CreateVolumeBackupPolicyAssignmentDetails
type CreateVolumeBackupPolicyAssignmentDetails struct {

	// The OCID of the asset (e.g. a volume) to which to assign the policy.
	AssetId *string `mandatory:"true" json:"assetId"`

	// The OCID of the volume backup policy to assign to an asset.
	PolicyId *string `mandatory:"true" json:"policyId"`
}

func (m CreateVolumeBackupPolicyAssignmentDetails) String() string {
	return common.PointerString(m)
}
