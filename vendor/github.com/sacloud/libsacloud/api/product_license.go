package api

// ProductLicenseAPI ライセンスプランAPI
type ProductLicenseAPI struct {
	*baseAPI
}

// NewProductLicenseAPI ライセンスプランAPI作成
func NewProductLicenseAPI(client *Client) *ProductLicenseAPI {
	return &ProductLicenseAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "product/license"
			},
		},
	}
}
