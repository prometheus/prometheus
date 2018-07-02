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

// IScsiVolumeAttachment An ISCSI volume attachment.
type IScsiVolumeAttachment struct {

	// The Availability Domain of an instance.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"true" json:"availabilityDomain"`

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The OCID of the volume attachment.
	Id *string `mandatory:"true" json:"id"`

	// The OCID of the instance the volume is attached to.
	InstanceId *string `mandatory:"true" json:"instanceId"`

	// The date and time the volume was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`

	// The OCID of the volume.
	VolumeId *string `mandatory:"true" json:"volumeId"`

	// The volume's iSCSI IP address.
	// Example: `169.254.0.2`
	Ipv4 *string `mandatory:"true" json:"ipv4"`

	// The target volume's iSCSI Qualified Name in the format defined by RFC 3720.
	// Example: `iqn.2015-12.us.oracle.com:456b0391-17b8-4122-bbf1-f85fc0bb97d9`
	Iqn *string `mandatory:"true" json:"iqn"`

	// The volume's iSCSI port.
	// Example: `3260`
	Port *int `mandatory:"true" json:"port"`

	// A user-friendly name. Does not have to be unique, and it cannot be changed.
	// Avoid entering confidential information.
	// Example: `My volume attachment`
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Whether the attachment was created in read-only mode.
	IsReadOnly *bool `mandatory:"false" json:"isReadOnly"`

	// The Challenge-Handshake-Authentication-Protocol (CHAP) secret valid for the associated CHAP user name.
	// (Also called the "CHAP password".)
	// Example: `d6866c0d-298b-48ba-95af-309b4faux45e`
	ChapSecret *string `mandatory:"false" json:"chapSecret"`

	// The volume's system-generated Challenge-Handshake-Authentication-Protocol (CHAP) user name.
	// Example: `ocid1.volume.oc1.phx.abyhqljrgvttnlx73nmrwfaux7kcvzfs3s66izvxf2h4lgvyndsdsnoiwr5q`
	ChapUsername *string `mandatory:"false" json:"chapUsername"`

	// The current state of the volume attachment.
	LifecycleState VolumeAttachmentLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`
}

//GetAvailabilityDomain returns AvailabilityDomain
func (m IScsiVolumeAttachment) GetAvailabilityDomain() *string {
	return m.AvailabilityDomain
}

//GetCompartmentId returns CompartmentId
func (m IScsiVolumeAttachment) GetCompartmentId() *string {
	return m.CompartmentId
}

//GetDisplayName returns DisplayName
func (m IScsiVolumeAttachment) GetDisplayName() *string {
	return m.DisplayName
}

//GetId returns Id
func (m IScsiVolumeAttachment) GetId() *string {
	return m.Id
}

//GetInstanceId returns InstanceId
func (m IScsiVolumeAttachment) GetInstanceId() *string {
	return m.InstanceId
}

//GetIsReadOnly returns IsReadOnly
func (m IScsiVolumeAttachment) GetIsReadOnly() *bool {
	return m.IsReadOnly
}

//GetLifecycleState returns LifecycleState
func (m IScsiVolumeAttachment) GetLifecycleState() VolumeAttachmentLifecycleStateEnum {
	return m.LifecycleState
}

//GetTimeCreated returns TimeCreated
func (m IScsiVolumeAttachment) GetTimeCreated() *common.SDKTime {
	return m.TimeCreated
}

//GetVolumeId returns VolumeId
func (m IScsiVolumeAttachment) GetVolumeId() *string {
	return m.VolumeId
}

func (m IScsiVolumeAttachment) String() string {
	return common.PointerString(m)
}

// MarshalJSON marshals to json representation
func (m IScsiVolumeAttachment) MarshalJSON() (buff []byte, e error) {
	type MarshalTypeIScsiVolumeAttachment IScsiVolumeAttachment
	s := struct {
		DiscriminatorParam string `json:"attachmentType"`
		MarshalTypeIScsiVolumeAttachment
	}{
		"iscsi",
		(MarshalTypeIScsiVolumeAttachment)(m),
	}

	return json.Marshal(&s)
}
