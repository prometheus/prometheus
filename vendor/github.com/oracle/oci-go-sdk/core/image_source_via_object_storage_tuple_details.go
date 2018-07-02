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

// ImageSourceViaObjectStorageTupleDetails The representation of ImageSourceViaObjectStorageTupleDetails
type ImageSourceViaObjectStorageTupleDetails struct {

	// The Object Storage bucket for the image.
	BucketName *string `mandatory:"true" json:"bucketName"`

	// The Object Storage namespace for the image.
	NamespaceName *string `mandatory:"true" json:"namespaceName"`

	// The Object Storage name for the image.
	ObjectName *string `mandatory:"true" json:"objectName"`

	// The format of the image to be imported.  Exported Oracle images are QCOW2.  Only monolithic
	// images are supported.
	SourceImageType ImageSourceDetailsSourceImageTypeEnum `mandatory:"false" json:"sourceImageType,omitempty"`
}

//GetSourceImageType returns SourceImageType
func (m ImageSourceViaObjectStorageTupleDetails) GetSourceImageType() ImageSourceDetailsSourceImageTypeEnum {
	return m.SourceImageType
}

func (m ImageSourceViaObjectStorageTupleDetails) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m ImageSourceViaObjectStorageTupleDetails) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeImageSourceViaObjectStorageTupleDetails ImageSourceViaObjectStorageTupleDetails
	s := struct {
		DiscriminatorParam string `json:"sourceType"`
		MarshalTypeImageSourceViaObjectStorageTupleDetails
	}{
		"objectStorageTuple",
		(MarshalTypeImageSourceViaObjectStorageTupleDetails)(m),
	}

	return json.Marshal(&s)
}
