package api

// ProductDiskAPI ディスクプランAPI
type ProductDiskAPI struct {
	*baseAPI
}

// NewProductDiskAPI ディスクプランAPI作成
func NewProductDiskAPI(client *Client) *ProductDiskAPI {
	return &ProductDiskAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "product/disk"
			},
		},
	}
}
