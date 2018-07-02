// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

// Core Services API
//
// APIs for Networking Service, Compute Service, and Block Volume Service.
//

package core

import (
	"encoding/json"
	"github.com/oracle/oci-go-sdk/common"
)

// VolumeGroupSourceFromVolumeGroupBackupDetails Specifies the volume group backup to restore from.
type VolumeGroupSourceFromVolumeGroupBackupDetails struct {

	// The OCID of the volume group backup to restore from.
	VolumeGroupBackupId *string `mandatory:"true" json:"volumeGroupBackupId"`
}

func (m VolumeGroupSourceFromVolumeGroupBackupDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m VolumeGroupSourceFromVolumeGroupBackupDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeVolumeGroupSourceFromVolumeGroupBackupDetails VolumeGroupSourceFromVolumeGroupBackupDetails
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeVolumeGroupSourceFromVolumeGroupBackupDetails
	}{
		"volumeGroupBackupId",
		(MarshalTypeVolumeGroupSourceFromVolumeGroupBackupDetails)(m),
	}

	return json.Marshal(&s)
}
