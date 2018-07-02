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

// BootVolumeSourceDetails The representation of BootVolumeSourceDetails
type BootVolumeSourceDetails interface {
}

type bootvolumesourcedetails struct {
	JsonData []byte
	Type     string `json:"type"`
}

// UnmarshalJSON unmarshals json
func (m *bootvolumesourcedetails) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalerbootvolumesourcedetails bootvolumesourcedetails
	s := struct {
		Model Unmarshalerbootvolumesourcedetails
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.Type = s.Model.Type

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *bootvolumesourcedetails) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {
	var err error
	switch m.Type {
	case "bootVolumeBackup":
		mm := BootVolumeSourceFromBootVolumeBackupDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "bootVolume":
		mm := BootVolumeSourceFromBootVolumeDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		return m, nil
	}
}

func (m bootvolumesourcedetails) String() string {
	return common.PointerString(m)
}
