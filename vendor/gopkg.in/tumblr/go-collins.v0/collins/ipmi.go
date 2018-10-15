package collins

// The IPMIService provides functions to work with the collins IP allocation
// manager.
//
// http://tumblr.github.io/collins/api.html#api-ipam-get-ipmi-address-pools
type IPMIService struct {
	client *Client
}

// IPAMService.IpmiPools return a list of the available IP pools for IPMI/OOB
// networks.
//
// http://tumblr.github.io/collins/api.html#api-ipam-get-ipmi-address-pools
func (s IPAMService) IpmiPools() ([]Pool, *Response, error) {
	ustr, err := addOptions("api/ipmi/pools", nil)
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
