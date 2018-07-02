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

// VolumeSourceDetails The representation of VolumeSourceDetails
type VolumeSourceDetails interface {
}

type volumesourcedetails struct {
	JsonData []byte
	Type     string `json:"type"`
}

// UnmarshalJSON unmarshals json
func (m *volumesourcedetails) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalervolumesourcedetails volumesourcedetails
	s := struct {
		Model Unmarshalervolumesourcedetails
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.Type = s.Model.Type

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *volumesourcedetails) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {
	var err error
	switch m.Type {
	case "volume":
		mm := VolumeSourceFromVolumeDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "volumeBackup":
		mm := VolumeSourceFromVolumeBackupDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		return m, nil
	}
}

func (m volumesourcedetails) String() string {
	return common.PointerString(m)
}
