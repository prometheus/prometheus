// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

// Core Services API
//
// APIs for Networking Service, Compute Service, and Block Volume Service.
//

package core

import (
	"github.com/oracle/oci-go-sdk/common"
)

// AttachBootVolumeDetails The representation of AttachBootVolumeDetails
type AttachBootVolumeDetails struct {

	// The OCID of the  boot volume.
	BootVolumeId *string `mandatory:"true" json:"bootVolumeId"`

	// The OCID of the instance.
	InstanceId *string `mandatory:"true" json:"instanceId"`

	// A user-friendly name. Does not have to be unique, and it cannot be changed. Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`
}

func (m AttachBootVolumeDetails) String() string {
	return common.PointerString(m)
}
