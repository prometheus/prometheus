package collins

// The IPAMService provides functions to work with the collins IP allocation
// manager.
//
// http://tumblr.github.io/collins/api.html#ipam
type IPAMService struct {
	client *Client
}

// Address represents an IPv4 address in the collins IPAM system.
type Address struct {
	Gateway string `json:"GATEWAY"`
	Address string `json:"ADDRESS"`
	Netmask string `json:"NETMASK"`
	Pool    string `json:"POOL"`
}

// A Pool represents a set of addresses with some common features and purpose.
type Pool struct {
	Name              string `json:"NAME"`
	Network           string `json:"NETWORK"`
	StartAddress      string `json:"START_ADDRESS"`
	SpecifiedGateway  string `json:"SPECIFIED_GATEWAY"`
	Gateway           string `json:"GATEWAY"`
	Broadcast         string `json:"BROADCAST"`
	PossibleAddresses int    `json:"POSSIBLE_ADDRESSES"`
}

// AddressAllocateOpts contain options that are available when allocating an
// address.
type AddressAllocateOpts struct {
	Count int    `url:"count,omitempty"`
	Pool  string `url:"pool,omitempty"`
}

// AddressUpdateOpts contain options that are available when updating an
// already allocated address.
type AddressUpdateOpts struct {
	Address    string `url:"address,omitempty"`
	Gateway    string `url:"gateway,omitempty"`
	Netmask    string `url:"netmask,omitempty"`
	Pool       string `url:"pool,omitempty"`
	OldAddress string `url:"old_address,omitempty"`
}

// AddressDeleteOpts contain options that are available when deleting an
// address.
type AddressDeleteOpts struct {
	Pool string `url:"pool,omitempty"`
}

// IPAMService.Allocate allocates one or more address to an asset. Pool can be
// specified as an option. Returns a list of the allocated addresses on success.
//
// http://tumblr.github.io/collins/api.html#api-ipam-allocate-an-address
func (s IPAMService) Allocate(tag string, opts AddressAllocateOpts) ([]Address, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/address", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("PUT", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Addresses []Address `json:"ADDRESSES"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Addresses, resp, nil
}

// IPAMService.Update updates an address associated with an asset.
//
// http://tumblr.github.io/collins/api.html#api-ipam-update-an-address
func (s IPAMService) Update(tag string, opts AddressUpdateOpts) (*Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/address", opts)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	return resp, err
}

// IPAMService.Delete deallocates all the addresses associated with an asset. If
// a pool name is passed in the options, only addresses in that pool will be
// deallocated.
//
// http://tumblr.github.io/collins/api.html#api-ipam-delete-an-address
func (s IPAMService) Delete(tag string, opts AddressDeleteOpts) (int, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/addresses", opts)
	if err != nil {
		return 0, nil, err
	}

	req, err := s.client.NewRequest("DELETE", ustr)
	if err != nil {
		return 0, nil, err
	}

	var c struct {
		Deleted int `json:"DELETED"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return 0, resp, err
	}

	return c.Deleted, resp, nil
}

// IPAMService.Pools return a list of the available IP pools.
//
// http://tumblr.github.io/collins/api.html#api-ipam-get-address-pools
func (s IPAMService) Pools() ([]Pool, *Response, error) {
	ustr, err := addOptions("api/address/pools", nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Pools []Pool `json:"POOLS"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Pools, resp, nil
}

// IPAMService.Get returns the addresses associated with a tag.
//
// http://tumblr.github.io/collins/api.html#api-ipam-asset-addresses
func (s IPAMService) Get(tag string) ([]Address, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/addresses", nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Addresses []Address `json:"ADDRESSES"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Addresses, resp, nil
}

// IPAMService.AssetFromAddress find the asset associated with the specified tag.
// Note that this will only return the metadata (tag, name, label etc.) of the
// asset. For a full asset use Asset.Get.
//
// http://tumblr.github.io/collins/api.html#api-ipam-asset-from-address
func (s IPAMService) AssetFromAddress(address string) (*Asset, *Response, error) {
	ustr, err := addOptions("api/asset/with/address/"+address, nil)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	metadata := new(Metadata)
	resp, err := s.client.Do(req, metadata)
	if err != nil {
		return nil, resp, err
	}

	asset := new(Asset)
	asset.Metadata = *metadata

	return asset, resp, nil
}
