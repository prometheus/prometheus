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

// CreateImageDetails Either instanceId or imageSourceDetails must be provided in addition to other required parameters.
type CreateImageDetails struct {

	// The OCID of the compartment containing the instance you want to use as the basis for the image.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name for the image. It does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	// You cannot use an Oracle-provided image name as a custom image name.
	// Example: `My Oracle Linux image`
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// Details for creating an image through import
	ImageSourceDetails ImageSourceDetails `mandatory:"false" json:"imageSourceDetails"`

	// The OCID of the instance you want to use as the basis for the image.
	InstanceId *string `mandatory:"false" json:"instanceId"`

	// Specifies the configuration mode for launching virtual machine (VM) instances. The configuration modes are:
	// * `NATIVE` - VM instances launch with iSCSI boot and VFIO devices. The default value for Oracle-provided images.
	// * `EMULATED` - VM instances launch with emulated devices, such as the E1000 network driver and emulated SCSI disk controller.
	// * `CUSTOM` - VM instances launch with custom configuration settings specified in the `LaunchOptions` parameter.
	LaunchMode CreateImageDetailsLaunchModeEnum `mandatory:"false" json:"launchMode,omitempty"`
}

func (m CreateImageDetails) String() string {
	return common.PointerString(m)
}

// UnmarshalJSON unmarshals from json
func (m *CreateImageDetails) UnmarshalJSON(data []byte) (e error) {
	model := struct {
		DefinedTags        map[string]map[string]interface{} `json:"definedTags"`
		DisplayName        *string                           `json:"displayName"`
		FreeformTags       map[string]string                 `json:"freeformTags"`
		ImageSourceDetails imagesourcedetails                `json:"imageSourceDetails"`
		InstanceId         *string                           `json:"instanceId"`
		LaunchMode         CreateImageDetailsLaunchModeEnum  `json:"launchMode"`
		CompartmentId      *string                           `json:"compartmentId"`
	}{}

	e = json.Unmarshal(data, &model)
	if e != nil {
		return
	}
	m.DefinedTags = model.DefinedTags
	m.DisplayName = model.DisplayName
	m.FreeformTags = model.FreeformTags
	nn, e := model.ImageSourceDetails.UnmarshalPolymorphicJSON(model.ImageSourceDetails.JsonData)
	if e != nil {
		return
	}
	m.ImageSourceDetails = nn.(ImageSourceDetails)
	m.InstanceId = model.InstanceId
	m.LaunchMode = model.LaunchMode
	m.CompartmentId = model.CompartmentId
	return
}

// CreateImageDetailsLaunchModeEnum Enum with underlying type: string
type CreateImageDetailsLaunchModeEnum string

// Set of constants representing the allowable values for CreateImageDetailsLaunchMode
const (
	CreateImageDetailsLaunchModeNative   CreateImageDetailsLaunchModeEnum = "NATIVE"
	CreateImageDetailsLaunchModeEmulated CreateImageDetailsLaunchModeEnum = "EMULATED"
	CreateImageDetailsLaunchModeCustom   CreateImageDetailsLaunchModeEnum = "CUSTOM"
)

var mappingCreateImageDetailsLaunchMode = map[string]CreateImageDetailsLaunchModeEnum{
	"NATIVE":   CreateImageDetailsLaunchModeNative,
	"EMULATED": CreateImageDetailsLaunchModeEmulated,
	"CUSTOM":   CreateImageDetailsLaunchModeCustom,
}

// GetCreateImageDetailsLaunchModeEnumValues Enumerates the set of values for CreateImageDetailsLaunchMode
func GetCreateImageDetailsLaunchModeEnumValues() []CreateImageDetailsLaunchModeEnum {
	values := make([]CreateImageDetailsLaunchModeEnum, 0)
	for _, v := range mappingCreateImageDetailsLaunchMode {
		values = append(values, v)
	}
	return values
}
