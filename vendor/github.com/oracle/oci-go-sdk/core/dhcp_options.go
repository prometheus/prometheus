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

// DhcpOptions A set of DHCP options. Used by the VCN to automatically provide configuration
// information to the instances when they boot up. There are two options you can set:
// - DhcpDnsOption: Lets you specify how DNS (hostname resolution) is
// handled in the subnets in your VCN.
// - DhcpSearchDomainOption: Lets you specify
// a search domain name to use for DNS queries.
// For more information, see  DNS in Your Virtual Cloud Network (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/dns.htm)
// and DHCP Options (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingDHCP.htm).
// To use any of the API operations, you must be authorized in an IAM policy. If you're not authorized,
// talk to an administrator. If you're an administrator who needs to write policies to give users access, see
// Getting Started with Policies (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/policygetstarted.htm).
type DhcpOptions struct {

	// The OCID of the compartment containing the set of DHCP options.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// Oracle ID (OCID) for the set of DHCP options.
	Id *string `mandatory:"true" json:"id"`

	// The current state of the set of DHCP options.
	LifecycleState DhcpOptionsLifecycleStateEnum `mandatory:"true" json:"lifecycleState"`

	// The collection of individual DHCP options.
	Options []DhcpOption `mandatory:"true" json:"options"`

	// Date and time the set of DHCP options was created, in the format defined by RFC3339.
	// Example: `2016-08-25T21:10:29.600Z`
	TimeCreated *common.SDKTime `mandatory:"true" json:"timeCreated"`

	// The OCID of the VCN the set of DHCP options belongs to.
	VcnId *string `mandatory:"true" json:"vcnId"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`
}

func (m DhcpOptions) String() string {
	return common.PointerString(m)
}

// UnmarshalJSON unmarshals from json
func (m *DhcpOptions) UnmarshalJSON(data []byte) (e error) {
	model := struct {
		DefinedTags    map[string]map[string]interface{} `json:"definedTags"`
		DisplayName    *string                           `json:"displayName"`
		FreeformTags   map[string]string                 `json:"freeformTags"`
		CompartmentId  *string                           `json:"compartmentId"`
		Id             *string                           `json:"id"`
		LifecycleState DhcpOptionsLifecycleStateEnum     `json:"lifecycleState"`
		Options        []dhcpoption                      `json:"options"`
		TimeCreated    *common.SDKTime                   `json:"timeCreated"`
		VcnId          *string                           `json:"vcnId"`
	}{}

	e = json.Unmarshal(data, &model)
	if e != nil {
		return
	}
	m.DefinedTags = model.DefinedTags
	m.DisplayName = model.DisplayName
	m.FreeformTags = model.FreeformTags
	m.CompartmentId = model.CompartmentId
	m.Id = model.Id
	m.LifecycleState = model.LifecycleState
	m.Options = make([]DhcpOption, len(model.Options))
	for i, n := range model.Options {
		nn, err := n.UnmarshalPolymorphicJSON(n.JsonData)
		if err != nil {
			return err
		}
		m.Options[i] = nn.(DhcpOption)
	}
	m.TimeCreated = model.TimeCreated
	m.VcnId = model.VcnId
	return
}

// DhcpOptionsLifecycleStateEnum Enum with underlying type: string
type DhcpOptionsLifecycleStateEnum string

// Set of constants representing the allowable values for DhcpOptionsLifecycleState
const (
	DhcpOptionsLifecycleStateProvisioning DhcpOptionsLifecycleStateEnum = "PROVISIONING"
	DhcpOptionsLifecycleStateAvailable    DhcpOptionsLifecycleStateEnum = "AVAILABLE"
	DhcpOptionsLifecycleStateTerminating  DhcpOptionsLifecycleStateEnum = "TERMINATING"
	DhcpOptionsLifecycleStateTerminated   DhcpOptionsLifecycleStateEnum = "TERMINATED"
)

var mappingDhcpOptionsLifecycleState = map[string]DhcpOptionsLifecycleStateEnum{
	"PROVISIONING": DhcpOptionsLifecycleStateProvisioning,
	"AVAILABLE":    DhcpOptionsLifecycleStateAvailable,
	"TERMINATING":  DhcpOptionsLifecycleStateTerminating,
	"TERMINATED":   DhcpOptionsLifecycleStateTerminated,
}

// GetDhcpOptionsLifecycleStateEnumValues Enumerates the set of values for DhcpOptionsLifecycleState
func GetDhcpOptionsLifecycleStateEnumValues() []DhcpOptionsLifecycleStateEnum {
	values := make([]DhcpOptionsLifecycleStateEnum, 0)
	for _, v := range mappingDhcpOptionsLifecycleState {
		values = append(values, v)
	}
	return values
}
