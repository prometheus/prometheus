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

// CreateDhcpDetails The representation of CreateDhcpDetails
type CreateDhcpDetails struct {

	// The OCID of the compartment to contain the set of DHCP options.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// A set of DHCP options.
	Options []DhcpOption `mandatory:"true" json:"options"`

	// The OCID of the VCN the set of DHCP options belongs to.
	VcnId *string `mandatory:"true" json:"vcnId"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable. Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`
}

func (m CreateDhcpDetails) String() string {
	return common.PointerString(m)
}

// UnmarshalJSON unmarshals from json
func (m *CreateDhcpDetails) UnmarshalJSON(data []byte) (e error) {
	model := struct {
		DefinedTags   map[string]map[string]interface{} `json:"definedTags"`
		DisplayName   *string                           `json:"displayName"`
		FreeformTags  map[string]string                 `json:"freeformTags"`
		CompartmentId *string                           `json:"compartmentId"`
		Options       []dhcpoption                      `json:"options"`
		VcnId         *string                           `json:"vcnId"`
	}{}

	e = json.Unmarshal(data, &model)
	if e != nil {
		return
	}
	m.DefinedTags = model.DefinedTags
	m.DisplayName = model.DisplayName
	m.FreeformTags = model.FreeformTags
	m.CompartmentId = model.CompartmentId
	m.Options = make([]DhcpOption, len(model.Options))
	for i, n := range model.Options {
		nn, err := n.UnmarshalPolymorphicJSON(n.JsonData)
		if err != nil {
			return err
		}
		m.Options[i] = nn.(DhcpOption)
	}
	m.VcnId = model.VcnId
	return
}
