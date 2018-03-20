package api

import (
	"encoding/json"
	//	"strings"
	"github.com/sacloud/libsacloud/sacloud"
)

//HACK: さくらのAPI側仕様: CommonServiceItemsの内容によってJSONフォーマットが異なるため
//      DNS/GSLB/シンプル監視それぞれでリクエスト/レスポンスデータ型を定義する。

// SearchGSLBResponse GSLB検索レスポンス
type SearchGSLBResponse struct {
	// Total 総件数
	Total int `json:",omitempty"`
	// From ページング開始位置
	From int `json:",omitempty"`
	// Count 件数
	Count int `json:",omitempty"`
	// CommonServiceGSLBItems GSLBリスト
	CommonServiceGSLBItems []sacloud.GSLB `json:"CommonServiceItems,omitempty"`
}

type gslbRequest struct {
	CommonServiceGSLBItem *sacloud.GSLB          `json:"CommonServiceItem,omitempty"`
	From                  int                    `json:",omitempty"`
	Count                 int                    `json:",omitempty"`
	Sort                  []string               `json:",omitempty"`
	Filter                map[string]interface{} `json:",omitempty"`
	Exclude               []string               `json:",omitempty"`
	Include               []string               `json:",omitempty"`
}

type gslbResponse struct {
	*sacloud.ResultFlagValue
	*sacloud.GSLB `json:"CommonServiceItem,omitempty"`
}

// GSLBAPI GSLB API
type GSLBAPI struct {
	*baseAPI
}

// NewGSLBAPI GSLB API作成
func NewGSLBAPI(client *Client) *GSLBAPI {
	return &GSLBAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "commonserviceitem"
			},
			FuncBaseSearchCondition: func() *sacloud.Request {
				res := &sacloud.Request{}
				res.AddFilter("Provider.Class", "gslb")
				return res
			},
		},
	}
}

// Find 検索
func (api *GSLBAPI) Find() (*SearchGSLBResponse, error) {

	data, err := api.client.newRequest("GET", api.getResourceURL(), api.getSearchState())
	if err != nil {
		return nil, err
	}
	var res SearchGSLBResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (api *GSLBAPI) request(f func(*gslbResponse) error) (*sacloud.GSLB, error) {
	res := &gslbResponse{}
	err := f(res)
	if err != nil {
		return nil, err
	}
	return res.GSLB, nil
}

func (api *GSLBAPI) createRequest(value *sacloud.GSLB) *gslbResponse {
	return &gslbResponse{GSLB: value}
}

// New 新規作成用パラメーター作成
func (api *GSLBAPI) New(name string) *sacloud.GSLB {
	return sacloud.CreateNewGSLB(name)
}

// Create 新規作成
func (api *GSLBAPI) Create(value *sacloud.GSLB) (*sacloud.GSLB, error) {
	return api.request(func(res *gslbResponse) error {
		return api.create(api.createRequest(value), res)
	})
}

// Read 読み取り
func (api *GSLBAPI) Read(id int64) (*sacloud.GSLB, error) {
	return api.request(func(res *gslbResponse) error {
		return api.read(id, nil, res)
	})
}

// Update 更新
func (api *GSLBAPI) Update(id int64, value *sacloud.GSLB) (*sacloud.GSLB, error) {
	return api.request(func(res *gslbResponse) error {
		return api.update(id, api.createRequest(value), res)
	})
}

// Delete 削除
func (api *GSLBAPI) Delete(id int64) (*sacloud.GSLB, error) {
	return api.request(func(res *gslbResponse) error {
		return api.delete(id, nil, res)
	})
}

// SetupGSLBRecord GSLB配下にサーバー追加
func (api *GSLBAPI) SetupGSLBRecord(gslbName string, ip string) ([]string, error) {

	gslbItem, err := api.findOrCreateBy(gslbName)

	if err != nil {
		return nil, err
	}
	gslbItem.Settings.GSLB.AddServer(ip)
	res, err := api.updateGSLBServers(gslbItem)
	if err != nil {
		return nil, err
	}

	if gslbItem.ID == sacloud.EmptyID {
		return []string{res.Status.FQDN}, nil
	}
	return nil, nil

}

// DeleteGSLBServer GSLB配下のサーバー削除
func (api *GSLBAPI) DeleteGSLBServer(gslbName string, ip string) error {
	gslbItem, err := api.findOrCreateBy(gslbName)
	if err != nil {
		return err
	}
	gslbItem.Settings.GSLB.DeleteServer(ip)

	if gslbItem.HasGSLBServer() {
		_, err = api.updateGSLBServers(gslbItem)
		if err != nil {
			return err
		}

	} else {
		_, err = api.Delete(gslbItem.ID)
		if err != nil {
			return err
		}

	}
	return nil
}

func (api *GSLBAPI) findOrCreateBy(gslbName string) (*sacloud.GSLB, error) {

	req := &sacloud.Request{}
	req.AddFilter("Name", gslbName)
	res, err := api.Find()
	if err != nil {
		return nil, err
	}

	//すでに登録されている場合
	var gslbItem *sacloud.GSLB
	if res.Count > 0 {
		gslbItem = &res.CommonServiceGSLBItems[0]
	} else {
		gslbItem = sacloud.CreateNewGSLB(gslbName)
	}

	return gslbItem, nil
}

func (api *GSLBAPI) updateGSLBServers(gslbItem *sacloud.GSLB) (*sacloud.GSLB, error) {

	var item *sacloud.GSLB
	var err error

	if gslbItem.ID == sacloud.EmptyID {
		item, err = api.Create(gslbItem)
	} else {
		item, err = api.Update(gslbItem.ID, gslbItem)
	}

	if err != nil {
		return nil, err
	}

	return item, nil
}
