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

// AttachParavirtualizedVolumeDetails The representation of AttachParavirtualizedVolumeDetails
type AttachParavirtualizedVolumeDetails struct {

	// The OCID of the instance.
	InstanceId *string `mandatory:"true" json:"instanceId"`

	// The OCID of the volume.
	VolumeId *string `mandatory:"true" json:"volumeId"`

	// A user-friendly name. Does not have to be unique, and it cannot be changed. Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Whether the attachment was created in read-only mode.
	IsReadOnly *bool `mandatory:"false" json:"isReadOnly"`
}

//GetDisplayName returns DisplayName
func (m AttachParavirtualizedVolumeDetails) GetDisplayName() *string {
	return m.DisplayName
}

//GetInstanceId returns InstanceId
func (m AttachParavirtualizedVolumeDetails) GetInstanceId() *string {
	return m.InstanceId
}

//GetIsReadOnly returns IsReadOnly
func (m AttachParavirtualizedVolumeDetails) GetIsReadOnly() *bool {
	return m.IsReadOnly
}

//GetVolumeId returns VolumeId
func (m AttachParavirtualizedVolumeDetails) GetVolumeId() *string {
	return m.VolumeId
}

func (m AttachParavirtualizedVolumeDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m AttachParavirtualizedVolumeDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeAttachParavirtualizedVolumeDetails AttachParavirtualizedVolumeDetails
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeAttachParavirtualizedVolumeDetails
	}{
		"paravirtualized",
		(MarshalTypeAttachParavirtualizedVolumeDetails)(m),
	}

	return json.Marshal(&s)
}
