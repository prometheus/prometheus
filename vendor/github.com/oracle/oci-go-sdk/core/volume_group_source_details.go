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

// VolumeGroupSourceDetails Specifies the source for a volume group.
type VolumeGroupSourceDetails interface {
}

type volumegroupsourcedetails struct {
	JsonData []byte
	Type     string `json:"type"`
}

// UnmarshalJSON unmarshals json
func (m *volumegroupsourcedetails) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalervolumegroupsourcedetails volumegroupsourcedetails
	s := struct {
		Model Unmarshalervolumegroupsourcedetails
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.Type = s.Model.Type

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *volumegroupsourcedetails) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {
	var err error
	switch m.Type {
	case "volumeGroupId":
		mm := VolumeGroupSourceFromVolumeGroupDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "volumeIds":
		mm := VolumeGroupSourceFromVolumesDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "volumeGroupBackupId":
		mm := VolumeGroupSourceFromVolumeGroupBackupDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		return m, nil
	}
}

func (m volumegroupsourcedetails) String() string {
	return common.PointerString(m)
}
