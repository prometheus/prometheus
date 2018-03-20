package api

// ProductInternetAPI ルータープランAPI
type ProductInternetAPI struct {
	*baseAPI
}

// NewProductInternetAPI ルータープランAPI作成
func NewProductInternetAPI(client *Client) *ProductInternetAPI {
	return &ProductInternetAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "product/internet"
			},
		},
	}
}
