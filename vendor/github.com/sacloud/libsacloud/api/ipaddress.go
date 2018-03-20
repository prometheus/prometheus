package api

import (
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
)

// IPAddressAPI IPアドレスAPI
type IPAddressAPI struct {
	*baseAPI
}

// NewIPAddressAPI IPアドレスAPI新規作成
func NewIPAddressAPI(client *Client) *IPAddressAPI {
	return &IPAddressAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "ipaddress"
			},
		},
	}
}

// Read 読み取り
func (api *IPAddressAPI) Read(ip string) (*sacloud.IPAddress, error) {
	return api.request(func(res *sacloud.Response) error {
		var (
			method = "GET"
			uri    = fmt.Sprintf("%s/%s", api.getResourceURL(), ip)
		)

		return api.baseAPI.request(method, uri, nil, res)
	})

}

// Update 更新(ホスト名逆引き設定)
func (api *IPAddressAPI) Update(ip string, hostName string) (*sacloud.IPAddress, error) {

	type request struct {
		// IPAddress
		IPAddress map[string]string
	}

	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%s", api.getResourceURL(), ip)
		body   = &request{IPAddress: map[string]string{}}
	)
	body.IPAddress["HostName"] = hostName

	return api.request(func(res *sacloud.Response) error {
		return api.baseAPI.request(method, uri, body, res)
	})
}
