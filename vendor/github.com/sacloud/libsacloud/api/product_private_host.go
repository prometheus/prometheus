package api

// ProductPrivateHostAPI 専有ホストプランAPI
type ProductPrivateHostAPI struct {
	*baseAPI
}

// NewProductPrivateHostAPI 専有ホストプランAPI作成
func NewProductPrivateHostAPI(client *Client) *ProductPrivateHostAPI {
	return &ProductPrivateHostAPI{
		&baseAPI{
			client: client,
			// FuncGetResourceURL
			FuncGetResourceURL: func() string {
				return "product/privatehost"
			},
		},
	}
}
