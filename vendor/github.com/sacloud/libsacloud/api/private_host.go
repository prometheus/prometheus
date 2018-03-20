package api

// PrivateHostAPI 専有ホストAPI
type PrivateHostAPI struct {
	*baseAPI
}

// NewPrivateHostAPI 専有ホストAPI作成
func NewPrivateHostAPI(client *Client) *PrivateHostAPI {
	return &PrivateHostAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "privatehost"
			},
		},
	}
}
