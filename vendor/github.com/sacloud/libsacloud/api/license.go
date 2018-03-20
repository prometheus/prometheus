package api

// LicenseAPI ライセンスAPI
type LicenseAPI struct {
	*baseAPI
}

// NewLicenseAPI ライセンスAPI作成
func NewLicenseAPI(client *Client) *LicenseAPI {
	return &LicenseAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "license"
			},
		},
	}
}
