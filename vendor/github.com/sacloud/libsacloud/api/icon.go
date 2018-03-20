package api

import "github.com/sacloud/libsacloud/sacloud"

// IconAPI アイコンAPI
type IconAPI struct {
	*baseAPI
}

// NewIconAPI アイコンAPI作成
func NewIconAPI(client *Client) *IconAPI {
	return &IconAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "icon"
			},
		},
	}
}

// GetImage アイコン画像データ(BASE64文字列)取得
func (api *IconAPI) GetImage(id int64, size string) (*sacloud.Image, error) {

	res := &sacloud.Response{}
	err := api.read(id, map[string]string{"Size": size}, res)
	if err != nil {
		return nil, err
	}
	return res.Image, nil
}
