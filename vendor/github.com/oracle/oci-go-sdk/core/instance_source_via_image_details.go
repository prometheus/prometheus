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

// InstanceSourceViaImageDetails The representation of InstanceSourceViaImageDetails
type InstanceSourceViaImageDetails struct {

	// The OCID of the image used to boot the instance.
	ImageId *string `mandatory:"true" json:"imageId"`

	// The size of the boot volume in GBs. Minimum value is 50 GB and maximum value is 16384 GB (16TB).
	BootVolumeSizeInGBs *int `mandatory:"false" json:"bootVolumeSizeInGBs"`
}

func (m InstanceSourceViaImageDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m InstanceSourceViaImageDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeInstanceSourceViaImageDetails InstanceSourceViaImageDetails
	s := struct {
		DiscriminatorParam string `json:"sourceType"`
		MarshalTypeInstanceSourceViaImageDetails
	}{
		"image",
		(MarshalTypeInstanceSourceViaImageDetails)(m),
	}

	return json.Marshal(&s)
}
