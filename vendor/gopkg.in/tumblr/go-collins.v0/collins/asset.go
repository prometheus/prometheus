package collins

import (
	"net/url"
)

// The AssetService provides functions to query, modify and manage assets in
// collins.
//
// http://tumblr.github.io/collins/api.html#asset
type AssetService struct {
	client *Client
}

// The Asset struct contains all the information available about a certain
// asset.
type Asset struct {
	Metadata       `json:"ASSET"`
	Hardware       `json:"HARDWARE"`
	Classification `json:"CLASSIFICATION"`
	Addresses      []Address `json:"ADDRESSES"`
	LLDP           `json:"LLDP"`
	IPMI           `json:"IPMI"`
	Attributes     map[string]map[string]string `json:"ATTRIBS"`
	Power          []Power                      `json:"POWER"`
}

// Metadata contains data about the asset such as asset tag, name, state and
// when it was last updated, among other things.
type Metadata struct {
	ID      int    `json:"ID"`
	Tag     string `json:"TAG"`
	Name    string `json:"NAME"`
	Label   string `json:"LABEL"`
	Type    string `json:"TYPE"`
	State   State  `json:"STATE"`
	Status  string `json:"STATUS"`
	Created string `json:"CREATED"`
	Updated string `json:"UPDATED"`
	Deleted string `json:"DELETED"`
}

// The Hardware struct collects information about the hardware configuration of
// an asset.
type Hardware struct {
	CPUs   []CPU    `json:"CPU"`
	Memory []Memory `json:"MEMORY"`
	NICs   []NIC    `json:"NIC"`
	Disks  []Disk   `json:"DISK"`
	Base   `json:"BASE"`
}

// The Classificaton struct contains information about the asset's classification.
type Classification struct {
	ID      int    `json:"ID"`
	Tag     string `json:"TAG"`
	State   State  `json:"STATE"`
	Status  string `json:"STATUS"`
	Type    string `json:"TYPE"`
	Created string `json:"CREATED"`
	Updated string `json:"UPDATED"`
	Deleted string `json:"DELETED"`
}

// CPU represents a single physical CPU on an asset.
type CPU struct {
	Cores       int     `json:"CORES"`
	Threads     int     `json:"THREADS"`
	SpeedGhz    float32 `json:"SPEED_GHZ"`
	Description string  `json:"DESCRIPTION"`
	Product     string  `json:"PRODUCT"`
	Vendor      string  `json:"VENDOR"`
}

// Memory represents a single memory module installed in a bank on the asset.
type Memory struct {
	Size        int    `json:"SIZE"`
	SizeHuman   string `json:"SIZE_HUMAN"`
	Bank        int    `json:"BANK"`
	Description string `json:"DESCRIPTION"`
	Product     string `json:"PRODUCT"`
	Vendor      string `json:"VENDOR"`
}

// The NIC struct represents a network interface card installed on the asset.
type NIC struct {
	Speed       int    `json:"SPEED"`
	SpeedHuman  string `json:"SPEED_HUMAN"`
	MacAddress  string `json:"MAC_ADDRESS"`
	Description string `json:"DESCRIPTION"`
	Product     string `json:"PRODUCT"`
	Vendor      string `json:"VENDOR"`
}

// Disk contains information about a disk drive installed in the asset.
type Disk struct {
	Size        int    `json:"SIZE"`
	SizeHuman   string `json:"SIZE_HUMAN"`
	Type        string `json:"TYPE"`
	Description string `json:"DESCRIPTION"`
	Product     string `json:"PRODUCT"`
	Vendor      string `json:"VENDOR"`
}

// The Base struct contains information about the vendor and product name of the
// asset.
type Base struct {
	Description string `json:"DESCRIPTION"`
	Product     string `json:"PRODUCT"`
	Vendor      string `json:"VENDOR"`
	Serial      string `json:"SERIAL"`
}

// The LLDP struct presents information that has been collected from the network
// interfaces of an asset using the LLDP protocol.
type LLDP struct {
	Interfaces []Interface `json:"INTERFACES"`
}

// Interface represents a network interface on the asset, complete with
// information about the switch it is connected to and the VLAN in use, as
// collected by LLDP.
type Interface struct {
	Name    string `json:"NAME"`
	Chassis struct {
		Name string `json:"NAME"`
		ID   struct {
			Type  string `json:"TYPE"`
			Value string `json:"VALUE"`
		} `json:"ID"`
		Description string `json:"DESCRIPTION"`
	} `json:"CHASSIS"`
	Port struct {
		ID struct {
			Type  string `json:"TYPE"`
			Value string `json:"VALUE"`
		} `json:"ID"`
		Description string `json:"DESCRIPTION"`
	} `json:"PORT"`
	Vlans []struct {
		ID   int    `json:"ID"`
		Name string `json:"NAME"`
	} `json:"VLANS"`
}

// IPMI contains the information the IPMI card on an asset is configured with.
type IPMI struct {
	ID       int    `json:"ID"`
	Username string `json:"IPMI_USERNAME"`
	Password string `json:"IPMI_PASSWORD"`
	Gateway  string `json:"IPMI_GATEWAY"`
	Address  string `json:"IPMI_ADDRESS"`
	Netmask  string `json:"IPMI_NETMASK"`
}

// The Power struct contains a list of power units detailing where the asset is
// connected to electrical power.
type Power struct {
	Units []PowerUnit `json:"UNITS"`
}

// A PowerUnit represents a single PDU on an asset, and where it is connected.
type PowerUnit struct {
	Key      string `json:"KEY"`
	Value    string `json:"VALUE"`
	Type     string `json:"TYPE"`
	Label    string `json:"LABEL"`
	Position int    `json:"POSITION"`
}

// AssetCreateOpts are options that must be passed when creating an asset
// (GenerateIPMI will default to false in the struct)
type AssetCreateOpts struct {
	GenerateIPMI bool   `url:"generate_ipmi,omitempty"`
	IpmiPool     string `url:"ipmi_pool,omitempty"`
	Status       string `url:"status,omitempty"`
	AssetType    string `url:"type,omitempty"`
}

// PowerConfig is used in AssetUpdateOpts to describe the power configuration of
// an asset. Since the keys and values are arbitrary the user are expected to
// add correct key/value pairs to the PowerConfig map.
type PowerConfig map[string]string

// PowerConfig.EncodeValues implements the Encoder interface from go-querystring
// in order to format the query the way collins expects it
func (pc PowerConfig) EncodeValues(key string, values *url.Values) error {
	for key, value := range pc {
		values.Add(key, value)
	}
	return nil
}

// AssetUpdateOpts are options to be passed when updating an asset
type AssetUpdateOpts struct {
	Attribute    string `url:"attribute,omitempty"`
	ChassisTag   string `url:"CHASSIS_TAG,omitempty"`
	GroupID      int    `url:"groupId,omitempty"`
	Lldp         string `url:"lldp,omitempty"`
	Lshw         string `url:"lshw,omitempty"`
	RackPosition string `url:"RACK_POSITION,omitempty"`
	PowerConfig
}

// AssetUpdateStatusOpts are options to be passed when updating asset status
type AssetUpdateStatusOpts struct {
	Reason string `url:"reason"`
	Status string `url:"status,omitempty"` // Optional arg
	State  string `url:"state,omitempty"`  // Optional arg
}

// AssetUpdateIPMIOpts are options that can be passed when updating the IPMI
// information about an asset.
type AssetUpdateIPMIOpts struct {
	Username string `url:"username,omitempty"`
	Password string `url:"password,omitempty"`
	Address  string `url:"address,omitempty"`
	Gateway  string `url:"gateway,omitempty"`
	Netmask  string `url:"netmask,omitempty"`
}

// AssetService.Create creates an asset with tag `tag`. Unless `generate_ipmi`
// is false, generate IPMI information.  If an `AssetType` is passed in as
// `asset_type`, use it to determine the type of the asset.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-create
func (s AssetService) Create(tag string, opts *AssetCreateOpts) (*Asset, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("PUT", ustr)
	if err != nil {
		return nil, nil, err
	}

	// Collins will pass back JSON with the created asset info, pass that
	// back along to client
	asset := &Asset{}
	resp, err := s.client.Do(req, asset)
	if err != nil {
		return nil, resp, err
	}

	return asset, resp, nil
}

// AssetService.Update update an asset with LSHW and LLDP info.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-update
func (s AssetService) Update(tag string, opts *AssetUpdateOpts) (*Response, error) {
	ustr, err := addOptions("api/asset/"+tag, opts)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// AssetService.Update updates the status of an asset.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-update-status
func (s AssetService) UpdateStatus(tag string, opts *AssetUpdateStatusOpts) (*Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/status", opts)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// AssetService.Get queries collins for an asset corresponding to tag passed
// as parameter.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-get
func (s AssetService) Get(tag string) (*Asset, *Response, error) {
	req, err := s.client.NewRequest("GET", "/api/asset/"+tag)
	if err != nil {
		return nil, nil, err
	}

	asset := &Asset{}
	resp, err := s.client.Do(req, asset)
	if err != nil {
		return nil, resp, err
	}

	return asset, resp, nil
}

// AssetService.Delete deletes an asset with the tag passed as parameter. It
// also requires a reason for the deletion.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-delete
func (s AssetService) Delete(tag, reason string) (*Response, error) {
	// Create url-encodable struct for request
	opts := struct {
		Reason string `url:"reason"`
	}{
		Reason: reason,
	}

	ustr, err := addOptions("api/asset/"+tag, opts)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("DELETE", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// AssetService.UpdateIpmi takes an asset tag and a AssetUpdateIPMIOpts struct
// as parameters, and updates the IPMI information accordingly.
//
// http://tumblr.github.io/collins/api.html#api-asset managment-ipmi-managment
func (s AssetService) UpdateIpmi(tag string, opts *AssetUpdateIPMIOpts) (*Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/ipmi", opts)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}
