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

// LaunchInstanceDetails Instance launch details.
// Use the `sourceDetails` parameter to specify whether a boot volume or an image should be used to launch a new instance.
type LaunchInstanceDetails struct {

	// The Availability Domain of the instance.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"true" json:"availabilityDomain"`

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" json:"compartmentId"`

	// The shape of an instance. The shape determines the number of CPUs, amount of memory,
	// and other resources allocated to the instance.
	// You can enumerate all available shapes by calling ListShapes.
	Shape *string `mandatory:"true" json:"shape"`

	// Details for the primary VNIC, which is automatically created and attached when
	// the instance is launched.
	CreateVnicDetails *CreateVnicDetails `mandatory:"false" json:"createVnicDetails"`

	// Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// For more information, see Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Operations": {"CostCenter": "42"}}`
	DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// A user-friendly name. Does not have to be unique, and it's changeable.
	// Avoid entering confidential information.
	// Example: `My bare metal instance`
	DisplayName *string `mandatory:"false" json:"displayName"`

	// Additional metadata key/value pairs that you provide.  They serve a similar purpose and functionality from fields in the 'metadata' object.
	// They are distinguished from 'metadata' fields in that these can be nested JSON objects (whereas 'metadata' fields are string/string maps only).
	// If you don't need nested metadata values, it is strongly advised to avoid using this object and use the Metadata object instead.
	ExtendedMetadata map[string]interface{} `mandatory:"false" json:"extendedMetadata"`

	// Free-form tags for this resource. Each tag is a simple key-value pair with no
	// predefined name, type, or namespace. For more information, see
	// Resource Tags (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/resourcetags.htm).
	// Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// Deprecated. Instead use `hostnameLabel` in
	// CreateVnicDetails.
	// If you provide both, the values must match.
	HostnameLabel *string `mandatory:"false" json:"hostnameLabel"`

	// Deprecated. Use `sourceDetails` with InstanceSourceViaImageDetails
	// source type instead. If you specify values for both, the values must match.
	ImageId *string `mandatory:"false" json:"imageId"`

	// This is an advanced option.
	// When a bare metal or virtual machine
	// instance boots, the iPXE firmware that runs on the instance is
	// configured to run an iPXE script to continue the boot process.
	// If you want more control over the boot process, you can provide
	// your own custom iPXE script that will run when the instance boots;
	// however, you should be aware that the same iPXE script will run
	// every time an instance boots; not only after the initial
	// LaunchInstance call.
	// The default iPXE script connects to the instance's local boot
	// volume over iSCSI and performs a network boot. If you use a custom iPXE
	// script and want to network-boot from the instance's local boot volume
	// over iSCSI the same way as the default iPXE script, you should use the
	// following iSCSI IP address: 169.254.0.2, and boot volume IQN:
	// iqn.2015-02.oracle.boot.
	// For more information about the Bring Your Own Image feature of
	// Oracle Cloud Infrastructure, see
	// Bring Your Own Image (https://docs.us-phoenix-1.oraclecloud.com/Content/Compute/References/bringyourownimage.htm).
	// For more information about iPXE, see http://ipxe.org.
	IpxeScript *string `mandatory:"false" json:"ipxeScript"`

	// Custom metadata key/value pairs that you provide, such as the SSH public key
	// required to connect to the instance.
	// A metadata service runs on every launched instance. The service is an HTTP
	// endpoint listening on 169.254.169.254. You can use the service to:
	// * Provide information to Cloud-Init (https://cloudinit.readthedocs.org/en/latest/)
	//   to be used for various system initialization tasks.
	// * Get information about the instance, including the custom metadata that you
	//   provide when you launch the instance.
	//  **Providing Cloud-Init Metadata**
	//  You can use the following metadata key names to provide information to
	//  Cloud-Init:
	//  **"ssh_authorized_keys"** - Provide one or more public SSH keys to be
	//  included in the `~/.ssh/authorized_keys` file for the default user on the
	//  instance. Use a newline character to separate multiple keys. The SSH
	//  keys must be in the format necessary for the `authorized_keys` file, as shown
	//  in the example below.
	//  **"user_data"** - Provide your own base64-encoded data to be used by
	//  Cloud-Init to run custom scripts or provide custom Cloud-Init configuration. For
	//  information about how to take advantage of user data, see the
	//  Cloud-Init Documentation (http://cloudinit.readthedocs.org/en/latest/topics/format.html).
	//  **Note:** Cloud-Init does not pull this data from the `http://169.254.169.254/opc/v1/instance/metadata/`
	//  path. When the instance launches and either of these keys are provided, the key values are formatted as
	//  OpenStack metadata and copied to the following locations, which are recognized by Cloud-Init:
	//  `http://169.254.169.254/openstack/latest/meta_data.json` - This JSON blob
	//  contains, among other things, the SSH keys that you provided for
	//   **"ssh_authorized_keys"**.
	//  `http://169.254.169.254/openstack/latest/user_data` - Contains the
	//  base64-decoded data that you provided for **"user_data"**.
	//  **Metadata Example**
	//       "metadata" : {
	//          "quake_bot_level" : "Severe",
	//          "ssh_authorized_keys" : "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCZ06fccNTQfq+xubFlJ5ZR3kt+uzspdH9tXL+lAejSM1NXM+CFZev7MIxfEjas06y80ZBZ7DUTQO0GxJPeD8NCOb1VorF8M4xuLwrmzRtkoZzU16umt4y1W0Q4ifdp3IiiU0U8/WxczSXcUVZOLqkz5dc6oMHdMVpkimietWzGZ4LBBsH/LjEVY7E0V+a0sNchlVDIZcm7ErReBLcdTGDq0uLBiuChyl6RUkX1PNhusquTGwK7zc8OBXkRuubn5UKXhI3Ul9Nyk4XESkVWIGNKmw8mSpoJSjR8P9ZjRmcZVo8S+x4KVPMZKQEor== ryan.smith@company.com
	//          ssh-rsa AAAAB3NzaC1yc2EAAAABJQAAAQEAzJSAtwEPoB3Jmr58IXrDGzLuDYkWAYg8AsLYlo6JZvKpjY1xednIcfEVQJm4T2DhVmdWhRrwQ8DmayVZvBkLt+zs2LdoAJEVimKwXcJFD/7wtH8Lnk17HiglbbbNXsemjDY0hea4JUE5CfvkIdZBITuMrfqSmA4n3VNoorXYdvtTMoGG8fxMub46RPtuxtqi9bG9Zqenordkg5FJt2mVNfQRqf83CWojcOkklUWq4CjyxaeLf5i9gv1fRoBo4QhiA8I6NCSppO8GnoV/6Ox6TNoh9BiifqGKC9VGYuC89RvUajRBTZSK2TK4DPfaT+2R+slPsFrwiT/oPEhhEK1S5Q== rsa-key-20160227",
	//          "user_data" : "SWYgeW91IGNhbiBzZWUgdGhpcywgdGhlbiBpdCB3b3JrZWQgbWF5YmUuCg=="
	//       }
	//  **Getting Metadata on the Instance**
	//  To get information about your instance, connect to the instance using SSH and issue any of the
	//  following GET requests:
	//      curl http://169.254.169.254/opc/v1/instance/
	//      curl http://169.254.169.254/opc/v1/instance/metadata/
	//      curl http://169.254.169.254/opc/v1/instance/metadata/<any-key-name>
	//  You'll get back a response that includes all the instance information; only the metadata information; or
	//  the metadata information for the specified key name, respectively.
	Metadata map[string]string `mandatory:"false" json:"metadata"`

	// Details for creating an instance.
	// Use this parameter to specify whether a boot volume or an image should be used to launch a new instance.
	SourceDetails InstanceSourceDetails `mandatory:"false" json:"sourceDetails"`

	// Deprecated. Instead use `subnetId` in
	// CreateVnicDetails.
	// At least one of them is required; if you provide both, the values must match.
	SubnetId *string `mandatory:"false" json:"subnetId"`
}

func (m LaunchInstanceDetails) String() string {
	return common.PointerString(m)
}

// UnmarshalJSON unmarshals from json
func (m *LaunchInstanceDetails) UnmarshalJSON(data []byte) (e error) {
	model := struct {
		CreateVnicDetails  *CreateVnicDetails                `json:"createVnicDetails"`
		DefinedTags        map[string]map[string]interface{} `json:"definedTags"`
		DisplayName        *string                           `json:"displayName"`
		ExtendedMetadata   map[string]interface{}            `json:"extendedMetadata"`
		FreeformTags       map[string]string                 `json:"freeformTags"`
		HostnameLabel      *string                           `json:"hostnameLabel"`
		ImageId            *string                           `json:"imageId"`
		IpxeScript         *string                           `json:"ipxeScript"`
		Metadata           map[string]string                 `json:"metadata"`
		SourceDetails      instancesourcedetails             `json:"sourceDetails"`
		SubnetId           *string                           `json:"subnetId"`
		AvailabilityDomain *string                           `json:"availabilityDomain"`
		CompartmentId      *string                           `json:"compartmentId"`
		Shape              *string                           `json:"shape"`
	}{}

	e = json.Unmarshal(data, &model)
	if e != nil {
		return
	}
	m.CreateVnicDetails = model.CreateVnicDetails
	m.DefinedTags = model.DefinedTags
	m.DisplayName = model.DisplayName
	m.ExtendedMetadata = model.ExtendedMetadata
	m.FreeformTags = model.FreeformTags
	m.HostnameLabel = model.HostnameLabel
	m.ImageId = model.ImageId
	m.IpxeScript = model.IpxeScript
	m.Metadata = model.Metadata
	nn, e := model.SourceDetails.UnmarshalPolymorphicJSON(model.SourceDetails.JsonData)
	if e != nil {
		return
	}
	m.SourceDetails = nn.(InstanceSourceDetails)
	m.SubnetId = model.SubnetId
	m.AvailabilityDomain = model.AvailabilityDomain
	m.CompartmentId = model.CompartmentId
	m.Shape = model.Shape
	return
}
