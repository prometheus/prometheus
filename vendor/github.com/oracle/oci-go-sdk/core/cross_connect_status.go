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

// CrossConnectStatus The status of the cross-connect.
type CrossConnectStatus struct {

	// The OCID of the cross-connect.
	CrossConnectId *string `mandatory:"true" json:"crossConnectId"`

	// Whether Oracle's side of the interface is up or down.
	InterfaceState CrossConnectStatusInterfaceStateEnum `mandatory:"false" json:"interfaceState,omitempty"`

	// The light level of the cross-connect (in dBm).
	// Example: `14.0`
	LightLevelIndBm *float32 `mandatory:"false" json:"lightLevelIndBm"`

	// Status indicator corresponding to the light level.
	//   * **NO_LIGHT:** No measurable light
	//   * **LOW_WARN:** There's measurable light but it's too low
	//   * **HIGH_WARN:** Light level is too high
	//   * **BAD:** There's measurable light but the signal-to-noise ratio is bad
	//   * **GOOD:** Good light level
	LightLevelIndicator CrossConnectStatusLightLevelIndicatorEnum `mandatory:"false" json:"lightLevelIndicator,omitempty"`
}

func (m CrossConnectStatus) String() string {
	return common.PointerString(m)
}

// CrossConnectStatusInterfaceStateEnum Enum with underlying type: string
type CrossConnectStatusInterfaceStateEnum string

// Set of constants representing the allowable values for CrossConnectStatusInterfaceState
const (
	CrossConnectStatusInterfaceStateUp   CrossConnectStatusInterfaceStateEnum = "UP"
	CrossConnectStatusInterfaceStateDown CrossConnectStatusInterfaceStateEnum = "DOWN"
)

var mappingCrossConnectStatusInterfaceState = map[string]CrossConnectStatusInterfaceStateEnum{
	"UP":   CrossConnectStatusInterfaceStateUp,
	"DOWN": CrossConnectStatusInterfaceStateDown,
}

// GetCrossConnectStatusInterfaceStateEnumValues Enumerates the set of values for CrossConnectStatusInterfaceState
func GetCrossConnectStatusInterfaceStateEnumValues() []CrossConnectStatusInterfaceStateEnum {
	values := make([]CrossConnectStatusInterfaceStateEnum, 0)
	for _, v := range mappingCrossConnectStatusInterfaceState {
		values = append(values, v)
	}
	return values
}

// CrossConnectStatusLightLevelIndicatorEnum Enum with underlying type: string
type CrossConnectStatusLightLevelIndicatorEnum string

// Set of constants representing the allowable values for CrossConnectStatusLightLevelIndicator
const (
	CrossConnectStatusLightLevelIndicatorNoLight  CrossConnectStatusLightLevelIndicatorEnum = "NO_LIGHT"
	CrossConnectStatusLightLevelIndicatorLowWarn  CrossConnectStatusLightLevelIndicatorEnum = "LOW_WARN"
	CrossConnectStatusLightLevelIndicatorHighWarn CrossConnectStatusLightLevelIndicatorEnum = "HIGH_WARN"
	CrossConnectStatusLightLevelIndicatorBad      CrossConnectStatusLightLevelIndicatorEnum = "BAD"
	CrossConnectStatusLightLevelIndicatorGood     CrossConnectStatusLightLevelIndicatorEnum = "GOOD"
)

var mappingCrossConnectStatusLightLevelIndicator = map[string]CrossConnectStatusLightLevelIndicatorEnum{
	"NO_LIGHT":  CrossConnectStatusLightLevelIndicatorNoLight,
	"LOW_WARN":  CrossConnectStatusLightLevelIndicatorLowWarn,
	"HIGH_WARN": CrossConnectStatusLightLevelIndicatorHighWarn,
	"BAD":       CrossConnectStatusLightLevelIndicatorBad,
	"GOOD":      CrossConnectStatusLightLevelIndicatorGood,
}

// GetCrossConnectStatusLightLevelIndicatorEnumValues Enumerates the set of values for CrossConnectStatusLightLevelIndicator
func GetCrossConnectStatusLightLevelIndicatorEnumValues() []CrossConnectStatusLightLevelIndicatorEnum {
	values := make([]CrossConnectStatusLightLevelIndicatorEnum, 0)
	for _, v := range mappingCrossConnectStatusLightLevelIndicator {
		values = append(values, v)
	}
	return values
}
