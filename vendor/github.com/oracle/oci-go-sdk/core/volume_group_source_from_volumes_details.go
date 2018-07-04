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

// VolumeGroupSourceFromVolumesDetails Specifies the volumes in a volume group.
type VolumeGroupSourceFromVolumesDetails struct {

	// OCIDs for the volumes in this volume group.
	VolumeIds []string `mandatory:"true" json:"volumeIds"`
}

func (m VolumeGroupSourceFromVolumesDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m VolumeGroupSourceFromVolumesDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeVolumeGroupSourceFromVolumesDetails VolumeGroupSourceFromVolumesDetails
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeVolumeGroupSourceFromVolumesDetails
	}{
		"volumeIds",
		(MarshalTypeVolumeGroupSourceFromVolumesDetails)(m),
	}

	return json.Marshal(&s)
}
