package api

// BridgeAPI ブリッジAPI
type BridgeAPI struct {
	*baseAPI
}

// NewBridgeAPI ブリッジAPI作成
func NewBridgeAPI(client *Client) *BridgeAPI {
	return &BridgeAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "bridge"
			},
		},
	}
}
