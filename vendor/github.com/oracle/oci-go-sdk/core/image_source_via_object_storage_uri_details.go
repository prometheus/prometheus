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

// ImageSourceViaObjectStorageUriDetails The representation of ImageSourceViaObjectStorageUriDetails
type ImageSourceViaObjectStorageUriDetails struct {

	// The Object Storage URL for the image.
	SourceUri *string `mandatory:"true" json:"sourceUri"`

	// The format of the image to be imported.  Exported Oracle images are QCOW2.  Only monolithic
	// images are supported.
	SourceImageType ImageSourceDetailsSourceImageTypeEnum `mandatory:"false" json:"sourceImageType,omitempty"`
}

//GetSourceImageType returns SourceImageType
func (m ImageSourceViaObjectStorageUriDetails) GetSourceImageType() ImageSourceDetailsSourceImageTypeEnum {
	return m.SourceImageType
}

func (m ImageSourceViaObjectStorageUriDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m ImageSourceViaObjectStorageUriDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeImageSourceViaObjectStorageUriDetails ImageSourceViaObjectStorageUriDetails
	s := struct {
		DiscriminatorParam string `json:"sourceType"`
		MarshalTypeImageSourceViaObjectStorageUriDetails
	}{
		"objectStorageUri",
		(MarshalTypeImageSourceViaObjectStorageUriDetails)(m),
	}

	return json.Marshal(&s)
}
