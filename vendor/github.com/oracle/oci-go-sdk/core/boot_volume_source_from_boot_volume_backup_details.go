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

// BootVolumeSourceFromBootVolumeBackupDetails Specifies the boot volume backup.
type BootVolumeSourceFromBootVolumeBackupDetails struct {

	// The OCID of the boot volume backup.
	Id *string `mandatory:"true" json:"id"`
}

func (m BootVolumeSourceFromBootVolumeBackupDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m BootVolumeSourceFromBootVolumeBackupDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeBootVolumeSourceFromBootVolumeBackupDetails BootVolumeSourceFromBootVolumeBackupDetails
	s := struct {
		DiscriminatorParam string `json:"type"`
		MarshalTypeBootVolumeSourceFromBootVolumeBackupDetails
	}{
		"bootVolumeBackup",
		(MarshalTypeBootVolumeSourceFromBootVolumeBackupDetails)(m),
	}

	return json.Marshal(&s)
}
