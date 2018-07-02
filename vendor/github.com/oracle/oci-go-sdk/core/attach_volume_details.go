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

// AttachVolumeDetails The representation of AttachVolumeDetails
type AttachVolumeDetails interface {

	// The OCID of the instance.
	GetInstanceId() *string

	// The OCID of the volume.
	GetVolumeId() *string

	// A user-friendly name. Does not have to be unique, and it cannot be changed. Avoid entering confidential information.
	GetDisplayName() *string

	// Whether the attachment was created in read-only mode.
	GetIsReadOnly() *bool
}

type attachvolumedetails struct {
	JsonData    []byte
	InstanceId  *string `mandatory:"true" json:"instanceId"`
	VolumeId    *string `mandatory:"true" json:"volumeId"`
	DisplayName *string `mandatory:"false" json:"displayName"`
	IsReadOnly  *bool   `mandatory:"false" json:"isReadOnly"`
	Type        string  `json:"type"`
}

// UnmarshalJSON unmarshals json
func (m *attachvolumedetails) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalerattachvolumedetails attachvolumedetails
	s := struct {
		Model Unmarshalerattachvolumedetails
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.InstanceId = s.Model.InstanceId
	m.VolumeId = s.Model.VolumeId
	m.DisplayName = s.Model.DisplayName
	m.IsReadOnly = s.Model.IsReadOnly
	m.Type = s.Model.Type

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *attachvolumedetails) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {
	var err error
	switch m.Type {
	case "iscsi":
		mm := AttachIScsiVolumeDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "paravirtualized":
		mm := AttachParavirtualizedVolumeDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		return m, nil
	}
}

//GetInstanceId returns InstanceId
func (m attachvolumedetails) GetInstanceId() *string {
	return m.InstanceId
}

//GetVolumeId returns VolumeId
func (m attachvolumedetails) GetVolumeId() *string {
	return m.VolumeId
}

//GetDisplayName returns DisplayName
func (m attachvolumedetails) GetDisplayName() *string {
	return m.DisplayName
}

//GetIsReadOnly returns IsReadOnly
func (m attachvolumedetails) GetIsReadOnly() *bool {
	return m.IsReadOnly
}

func (m attachvolumedetails) String() string {
	return common.PointerString(m)
}
