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

// ImageSourceDetails The representation of ImageSourceDetails
type ImageSourceDetails interface {

	// The format of the image to be imported.  Exported Oracle images are QCOW2.  Only monolithic
	// images are supported.
	GetSourceImageType() ImageSourceDetailsSourceImageTypeEnum
}

type imagesourcedetails struct {
	JsonData        []byte
	SourceImageType ImageSourceDetailsSourceImageTypeEnum `mandatory:"false" json:"sourceImageType,omitempty"`
	SourceType      string                                `json:"sourceType"`
}

// UnmarshalJSON unmarshals json
func (m *imagesourcedetails) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalerimagesourcedetails imagesourcedetails
	s := struct {
		Model Unmarshalerimagesourcedetails
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.SourceImageType = s.Model.SourceImageType
	m.SourceType = s.Model.SourceType

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *imagesourcedetails) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {
	var err error
	switch m.SourceType {
	case "objectStorageTuple":
		mm := ImageSourceViaObjectStorageTupleDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "objectStorageUri":
		mm := ImageSourceViaObjectStorageUriDetails{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		return m, nil
	}
}

//GetSourceImageType returns SourceImageType
func (m imagesourcedetails) GetSourceImageType() ImageSourceDetailsSourceImageTypeEnum {
	return m.SourceImageType
}

func (m imagesourcedetails) String() string {
	return common.PointerString(m)
}

// ImageSourceDetailsSourceImageTypeEnum Enum with underlying type: string
type ImageSourceDetailsSourceImageTypeEnum string

// Set of constants representing the allowable values for ImageSourceDetailsSourceImageType
const (
	ImageSourceDetailsSourceImageTypeQcow2 ImageSourceDetailsSourceImageTypeEnum = "QCOW2"
	ImageSourceDetailsSourceImageTypeVmdk  ImageSourceDetailsSourceImageTypeEnum = "VMDK"
)

var mappingImageSourceDetailsSourceImageType = map[string]ImageSourceDetailsSourceImageTypeEnum{
	"QCOW2": ImageSourceDetailsSourceImageTypeQcow2,
	"VMDK":  ImageSourceDetailsSourceImageTypeVmdk,
}

// GetImageSourceDetailsSourceImageTypeEnumValues Enumerates the set of values for ImageSourceDetailsSourceImageType
func GetImageSourceDetailsSourceImageTypeEnumValues() []ImageSourceDetailsSourceImageTypeEnum {
	values := make([]ImageSourceDetailsSourceImageTypeEnum, 0)
	for _, v := range mappingImageSourceDetailsSourceImageType {
		values = append(values, v)
	}
	return values
}
