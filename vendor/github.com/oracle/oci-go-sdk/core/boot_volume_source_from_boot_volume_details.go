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

// BootVolumeSourceFromBootVolumeDetails Specifies the source boot volume.
type BootVolumeSourceFromBootVolumeDetails struct {

	// The OCID of the boot volume.
	Id *string `mandatory:"true" json:"id"`
}

func (m BootVolumeSourceFromBootVolumeDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m BootVolumeSourceFromBootVolumeDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeBootVolumeSourceFromBootVolumeDetails BootVolumeSourceFromBootVolumeDetails
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeBootVolumeSourceFromBootVolumeDetails
	}{
		"bootVolume",
		(MarshalTypeBootVolumeSourceFromBootVolumeDetails)(m),
	}

	return json.Marshal(&s)
}
