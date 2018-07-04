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

// VolumeBackupSchedule Defines a chronological recurrence pattern for creating scheduled backups at a particular periodicity.
type VolumeBackupSchedule struct {

	// The type of backup to create.
	BackupType VolumeBackupScheduleBackupTypeEnum `mandatory:"true" json:"backupType"`

	// The number of seconds (positive or negative) that the backup time should be shifted from the default interval boundaries specified by the period.
	OffsetSeconds *int `mandatory:"true" json:"offsetSeconds"`

	// How often the backup should occur.
	Period VolumeBackupSchedulePeriodEnum `mandatory:"true" json:"period"`

	// How long, in seconds, backups created by this schedule should be kept until being automatically deleted.
	RetentionSeconds *int `mandatory:"true" json:"retentionSeconds"`
}

func (m VolumeBackupSchedule) String() string {
	return common.PointerString(m)
}

// VolumeBackupScheduleBackupTypeEnum Enum with underlying type: string
type VolumeBackupScheduleBackupTypeEnum string

// Set of constants representing the allowable values for VolumeBackupScheduleBackupType
const (
	VolumeBackupScheduleBackupTypeFull        VolumeBackupScheduleBackupTypeEnum = "FULL"
	VolumeBackupScheduleBackupTypeIncremental VolumeBackupScheduleBackupTypeEnum = "INCREMENTAL"
)

var mappingVolumeBackupScheduleBackupType = map[string]VolumeBackupScheduleBackupTypeEnum{
	"FULL":        VolumeBackupScheduleBackupTypeFull,
	"INCREMENTAL": VolumeBackupScheduleBackupTypeIncremental,
}

// GetVolumeBackupScheduleBackupTypeEnumValues Enumerates the set of values for VolumeBackupScheduleBackupType
func GetVolumeBackupScheduleBackupTypeEnumValues() []VolumeBackupScheduleBackupTypeEnum {
	values := make([]VolumeBackupScheduleBackupTypeEnum, 0)
	for _, v := range mappingVolumeBackupScheduleBackupType {
		values = append(values, v)
	}
	return values
}

// VolumeBackupSchedulePeriodEnum Enum with underlying type: string
type VolumeBackupSchedulePeriodEnum string

// Set of constants representing the allowable values for VolumeBackupSchedulePeriod
const (
	VolumeBackupSchedulePeriodHour  VolumeBackupSchedulePeriodEnum = "ONE_HOUR"
	VolumeBackupSchedulePeriodDay   VolumeBackupSchedulePeriodEnum = "ONE_DAY"
	VolumeBackupSchedulePeriodWeek  VolumeBackupSchedulePeriodEnum = "ONE_WEEK"
	VolumeBackupSchedulePeriodMonth VolumeBackupSchedulePeriodEnum = "ONE_MONTH"
	VolumeBackupSchedulePeriodYear  VolumeBackupSchedulePeriodEnum = "ONE_YEAR"
)

var mappingVolumeBackupSchedulePeriod = map[string]VolumeBackupSchedulePeriodEnum{
	"ONE_HOUR":  VolumeBackupSchedulePeriodHour,
	"ONE_DAY":   VolumeBackupSchedulePeriodDay,
	"ONE_WEEK":  VolumeBackupSchedulePeriodWeek,
	"ONE_MONTH": VolumeBackupSchedulePeriodMonth,
	"ONE_YEAR":  VolumeBackupSchedulePeriodYear,
}

// GetVolumeBackupSchedulePeriodEnumValues Enumerates the set of values for VolumeBackupSchedulePeriod
func GetVolumeBackupSchedulePeriodEnumValues() []VolumeBackupSchedulePeriodEnum {
	values := make([]VolumeBackupSchedulePeriodEnum, 0)
	for _, v := range mappingVolumeBackupSchedulePeriod {
		values = append(values, v)
	}
	return values
}
