package api

// RegionAPI リージョンAPI
type RegionAPI struct {
	*baseAPI
}

// NewRegionAPI リージョンAPI作成
func NewRegionAPI(client *Client) *RegionAPI {
	return &RegionAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "region"
			},
		},
	}
}
