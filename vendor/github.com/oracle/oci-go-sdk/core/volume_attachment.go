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

// VolumeAttachment A base object for all types of attachments between a storage volume and an instance.
// For specific details about iSCSI attachments, see
// IScsiVolumeAttachment.
// For general information about volume attachments, see
// Overview of Block Volume Storage (https://docs.us-phoenix-1.oraclecloud.com/Content/Block/Concepts/overview.htm).
type VolumeAttachment interface {

	// The Availability Domain of an instance.
	// Example: `Uocm:PHX-AD-1`
	GetAvailabilityDomain() *string

	// The OCID of the compartment.
	GetCompartmentId() *string

	// The OCID of the volume attachment.
	GetId() *string

	// The OCID of the instance the volume is attached to.
	GetInstanceId() *string

	// The current state of the volume attachment.
	GetLifecycleState() VolumeAttachmentLifecycleStateEnum

	// The date and time the volume was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	GetTimeCreated() *common.SDKTime

	// The OCID of the volume.
	GetVolumeId() *string

	// A user-friendly name. Does not have to be unique, and it cannot be changed.
	// Avoid entering confidential information.
	// Example: `My volume attachment`
	GetDisplayName() *string

	// Whether the attachment was created in read-only mode.
	GetIsReadOnly() *bool
}

type volumeattachment struct {
	JsonData           []byte
	AvailabilityDomain *string                            `mandatory:"true" json:"availabilityDomain"`
	CompartmentId      *string                            `mandatory:"true" json:"compartmentId"`
	Id                 *string                            `mandatory:"true" json:"id"`
	InstanceId         *string                            `mandatory:"true" json:"instanceId"`
	LifecycleState     VolumeAttachmentLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`
	TimeCreated        *common.SDKTime                    `mandatory:"true" json:"timeCreated"`
	VolumeId           *string                            `mandatory:"true" json:"volumeId"`
	DisplayName        *string                            `mandatory:"false" json:"displayName"`
	IsReadOnly         *bool                              `mandatory:"false" json:"isReadOnly"`
	AttachmentType     string                             `json:"attachmentType"`
}

// UnmarshalJSON unmarshals json
func (m *volumeattachment) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalervolumeattachment volumeattachment
	s := struct {
		Model Unmarshalervolumeattachment
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.AvailabilityDomain = s.Model.AvailabilityDomain
	m.CompartmentId = s.Model.CompartmentId
	m.Id = s.Model.Id
	m.InstanceId = s.Model.InstanceId
	m.LifecycleState = s.Model.LifecycleState
	m.TimeCreated = s.Model.TimeCreated
	m.VolumeId = s.Model.VolumeId
	m.DisplayName = s.Model.DisplayName
	m.IsReadOnly = s.Model.IsReadOnly
	m.AttachmentType = s.Model.AttachmentType

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *volumeattachment) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {
	var err error
	switch m.AttachmentType {
	case "iscsi":
		mm := IScsiVolumeAttachment{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "paravirtualized":
		mm := ParavirtualizedVolumeAttachment{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		return m, nil
	}
}

//GetAvailabilityDomain returns AvailabilityDomain
func (m volumeattachment) GetAvailabilityDomain() *string {
	return m.AvailabilityDomain
}

//GetCompartmentId returns CompartmentId
func (m volumeattachment) GetCompartmentId() *string {
	return m.CompartmentId
}

//GetId returns Id
func (m volumeattachment) GetId() *string {
	return m.Id
}

//GetInstanceId returns InstanceId
func (m volumeattachment) GetInstanceId() *string {
	return m.InstanceId
}

//GetLifecycleState returns LifecycleState
func (m volumeattachment) GetLifecycleState() VolumeAttachmentLifecycleStateEnum {
	return m.LifecycleState
}

//GetTimeCreated returns TimeCreated
func (m volumeattachment) GetTimeCreated() *common.SDKTime {
	return m.TimeCreated
}

//GetVolumeId returns VolumeId
func (m volumeattachment) GetVolumeId() *string {
	return m.VolumeId
}

//GetDisplayName returns DisplayName
func (m volumeattachment) GetDisplayName() *string {
	return m.DisplayName
}

//GetIsReadOnly returns IsReadOnly
func (m volumeattachment) GetIsReadOnly() *bool {
	return m.IsReadOnly
}

func (m volumeattachment) String() string {
	return common.PointerString(m)
}

// VolumeAttachmentLifecycleStateEnum Enum with underlying type: string
type VolumeAttachmentLifecycleStateEnum string

// Set of constants representing the allowable values for VolumeAttachmentLifecycleState
const (
	VolumeAttachmentLifecycleStateAttaching VolumeAttachmentLifecycleStateEnum = "ATTACHING"
	VolumeAttachmentLifecycleStateAttached  VolumeAttachmentLifecycleStateEnum = "ATTACHED"
	VolumeAttachmentLifecycleStateDetaching VolumeAttachmentLifecycleStateEnum = "DETACHING"
	VolumeAttachmentLifecycleStateDetached  VolumeAttachmentLifecycleStateEnum = "DETACHED"
)

var mappingVolumeAttachmentLifecycleState = map[string]VolumeAttachmentLifecycleStateEnum{
	"ATTACHING": VolumeAttachmentLifecycleStateAttaching,
	"ATTACHED":  VolumeAttachmentLifecycleStateAttached,
	"DETACHING": VolumeAttachmentLifecycleStateDetaching,
	"DETACHED":  VolumeAttachmentLifecycleStateDetached,
}

// GetVolumeAttachmentLifecycleStateEnumValues Enumerates the set of values for VolumeAttachmentLifecycleState
func GetVolumeAttachmentLifecycleStateEnumValues() []VolumeAttachmentLifecycleStateEnum {
	values := make([]VolumeAttachmentLifecycleStateEnum, 0)
	for _, v := range mappingVolumeAttachmentLifecycleState {
		values = append(values, v)
	}
	return values
}
