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

// VolumeSourceFromVolumeDetails Specifies the source volume.
type VolumeSourceFromVolumeDetails struct {

	// The OCID of the volume.
	Id *string `mandatory:"true" json:"id"`
}

func (m VolumeSourceFromVolumeDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m VolumeSourceFromVolumeDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeVolumeSourceFromVolumeDetails VolumeSourceFromVolumeDetails
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeVolumeSourceFromVolumeDetails
	}{
		"volume",
		(MarshalTypeVolumeSourceFromVolumeDetails)(m),
	}

	return json.Marshal(&s)
}
