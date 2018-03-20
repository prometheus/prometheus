package api

// IPv6NetAPI IPv6ネットワークAPI
type IPv6NetAPI struct {
	*baseAPI
}

// NewIPv6NetAPI IPv6ネットワークAPI作成
func NewIPv6NetAPI(client *Client) *IPv6NetAPI {
	return &IPv6NetAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "ipv6net"
			},
		},
	}
}
