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

// LetterOfAuthority The Letter of Authority for the cross-connect. You must submit this letter when
// requesting cabling for the cross-connect at the FastConnect location.
type LetterOfAuthority struct {

	// The name of the entity authorized by this Letter of Authority.
	AuthorizedEntityName *string `mandatory:"false" json:"authorizedEntityName"`

	// The type of cross-connect fiber, termination, and optical specification.
	CircuitType LetterOfAuthorityCircuitTypeEnum `mandatory:"false" json:"circuitType,omitempty"`

	// The OCID of the cross-connect.
	CrossConnectId *string `mandatory:"false" json:"crossConnectId"`

	// The address of the FastConnect location.
	FacilityLocation *string `mandatory:"false" json:"facilityLocation"`

	// The meet-me room port for this cross-connect.
	PortName *string `mandatory:"false" json:"portName"`

	// The date and time when the Letter of Authority expires, in the format defined by RFC3339.
	TimeExpires *common.SDKTime `mandatory:"false" json:"timeExpires"`

	// The date and time the Letter of Authority was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeIssued *common.SDKTime `mandatory:"false" json:"timeIssued"`
}

func (m LetterOfAuthority) String() string {
	return common.PointerString(m)
}

// LetterOfAuthorityCircuitTypeEnum Enum with underlying type: string
type LetterOfAuthorityCircuitTypeEnum string

// Set of constants representing the allowable values for LetterOfAuthorityCircuitType
const (
	LetterOfAuthorityCircuitTypeLc LetterOfAuthorityCircuitTypeEnum = "Single_mode_LC"
	LetterOfAuthorityCircuitTypeSc LetterOfAuthorityCircuitTypeEnum = "Single_mode_SC"
)

var mappingLetterOfAuthorityCircuitType = map[string]LetterOfAuthorityCircuitTypeEnum{
	"Single_mode_LC": LetterOfAuthorityCircuitTypeLc,
	"Single_mode_SC": LetterOfAuthorityCircuitTypeSc,
}

// GetLetterOfAuthorityCircuitTypeEnumValues Enumerates the set of values for LetterOfAuthorityCircuitType
func GetLetterOfAuthorityCircuitTypeEnumValues() []LetterOfAuthorityCircuitTypeEnum {
	values := make([]LetterOfAuthorityCircuitTypeEnum, 0)
	for _, v := range mappingLetterOfAuthorityCircuitType {
		values = append(values, v)
	}
	return values
}
