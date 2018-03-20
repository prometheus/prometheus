package api

// SubnetAPI サブネットAPI
type SubnetAPI struct {
	*baseAPI
}

// NewSubnetAPI サブネットAPI作成
func NewSubnetAPI(client *Client) *SubnetAPI {
	return &SubnetAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "subnet"
			},
		},
	}
}
