package api

// PublicPriceAPI 料金情報API
type PublicPriceAPI struct {
	*baseAPI
}

// NewPublicPriceAPI 料金情報API
func NewPublicPriceAPI(client *Client) *PublicPriceAPI {
	return &PublicPriceAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "public/price"
			},
		},
	}
}
