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

// VolumeBackupPolicy A policy for automatically creating volume backups according to a recurring schedule. Has a set of one or more schedules that control when and how backups are created.
type VolumeBackupPolicy struct {

	// A user-friendly name for the volume backup policy. Does not have to be unique and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"true" json:"displayName"`

	// The OCID of the volume backup policy.
	Id *string `mandatory:"true" json:"id"`

	// The collection of schedules that this policy will apply.
	Schedules []VolumeBackupSchedule `mandatory:"true" json:"schedules"`

	// The date and time the volume backup policy was created. Format defined by RFC3339.
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`
}

func (m VolumeBackupPolicy) String() string {
	return common.PointerString(m)
}
