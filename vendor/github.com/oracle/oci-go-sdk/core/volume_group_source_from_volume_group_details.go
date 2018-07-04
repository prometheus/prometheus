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

// VolumeGroupSourceFromVolumeGroupDetails Specifies the volume group to clone from.
type VolumeGroupSourceFromVolumeGroupDetails struct {

	// The OCID of the volume group to clone from.
	VolumeGroupId *string `mandatory:"true" json:"volumeGroupId"`
}

func (m VolumeGroupSourceFromVolumeGroupDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m VolumeGroupSourceFromVolumeGroupDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeVolumeGroupSourceFromVolumeGroupDetails VolumeGroupSourceFromVolumeGroupDetails
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeVolumeGroupSourceFromVolumeGroupDetails
	}{
		"volumeGroupId",
		(MarshalTypeVolumeGroupSourceFromVolumeGroupDetails)(m),
	}

	return json.Marshal(&s)
}
