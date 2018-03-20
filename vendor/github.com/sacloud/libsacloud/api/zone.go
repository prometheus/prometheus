package api

// ZoneAPI ゾーンAPI
type ZoneAPI struct {
	*baseAPI
}

// NewZoneAPI ゾーンAPI作成
func NewZoneAPI(client *Client) *ZoneAPI {
	return &ZoneAPI{
		&baseAPI{
			client: client,
			// FuncGetResourceURL
			FuncGetResourceURL: func() string {
				return "zone"
			},
		},
	}
}
