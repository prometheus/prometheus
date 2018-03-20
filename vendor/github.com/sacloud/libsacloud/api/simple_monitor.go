package api

import (
	"encoding/json"
	//	"strings"
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
)

//HACK: さくらのAPI側仕様: CommonServiceItemsの内容によってJSONフォーマットが異なるため
//      DNS/GSLB/シンプル監視それぞれでリクエスト/レスポンスデータ型を定義する。

// SearchSimpleMonitorResponse シンプル監視検索レスポンス
type SearchSimpleMonitorResponse struct {
	// Total 総件数
	Total int `json:",omitempty"`
	// From ページング開始位置
	From int `json:",omitempty"`
	// Count 件数
	Count int `json:",omitempty"`
	// SimpleMonitors シンプル監視 リスト
	SimpleMonitors []sacloud.SimpleMonitor `json:"CommonServiceItems,omitempty"`
}

type simpleMonitorRequest struct {
	SimpleMonitor *sacloud.SimpleMonitor `json:"CommonServiceItem,omitempty"`
	From          int                    `json:",omitempty"`
	Count         int                    `json:",omitempty"`
	Sort          []string               `json:",omitempty"`
	Filter        map[string]interface{} `json:",omitempty"`
	Exclude       []string               `json:",omitempty"`
	Include       []string               `json:",omitempty"`
}

type simpleMonitorResponse struct {
	*sacloud.ResultFlagValue
	*sacloud.SimpleMonitor `json:"CommonServiceItem,omitempty"`
}

// SimpleMonitorAPI シンプル監視API
type SimpleMonitorAPI struct {
	*baseAPI
}

// NewSimpleMonitorAPI シンプル監視API作成
func NewSimpleMonitorAPI(client *Client) *SimpleMonitorAPI {
	return &SimpleMonitorAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "commonserviceitem"
			},
			FuncBaseSearchCondition: func() *sacloud.Request {
				res := &sacloud.Request{}
				res.AddFilter("Provider.Class", "simplemon")
				return res
			},
		},
	}
}

// Find 検索
func (api *SimpleMonitorAPI) Find() (*SearchSimpleMonitorResponse, error) {
	data, err := api.client.newRequest("GET", api.getResourceURL(), api.getSearchState())
	if err != nil {
		return nil, err
	}
	var res SearchSimpleMonitorResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (api *SimpleMonitorAPI) request(f func(*simpleMonitorResponse) error) (*sacloud.SimpleMonitor, error) {
	res := &simpleMonitorResponse{}
	err := f(res)
	if err != nil {
		return nil, err
	}
	return res.SimpleMonitor, nil
}

func (api *SimpleMonitorAPI) createRequest(value *sacloud.SimpleMonitor) *simpleMonitorResponse {
	return &simpleMonitorResponse{SimpleMonitor: value}
}

// New 新規作成用パラメーター作成
func (api *SimpleMonitorAPI) New(target string) *sacloud.SimpleMonitor {
	return sacloud.CreateNewSimpleMonitor(target)
}

// Create 新規作成
func (api *SimpleMonitorAPI) Create(value *sacloud.SimpleMonitor) (*sacloud.SimpleMonitor, error) {
	return api.request(func(res *simpleMonitorResponse) error {
		return api.create(api.createRequest(value), res)
	})
}

// Read 読み取り
func (api *SimpleMonitorAPI) Read(id int64) (*sacloud.SimpleMonitor, error) {
	return api.request(func(res *simpleMonitorResponse) error {
		return api.read(id, nil, res)
	})
}

// Update 更新
func (api *SimpleMonitorAPI) Update(id int64, value *sacloud.SimpleMonitor) (*sacloud.SimpleMonitor, error) {
	return api.request(func(res *simpleMonitorResponse) error {
		return api.update(id, api.createRequest(value), res)
	})
}

// Delete 削除
func (api *SimpleMonitorAPI) Delete(id int64) (*sacloud.SimpleMonitor, error) {
	return api.request(func(res *simpleMonitorResponse) error {
		return api.delete(id, nil, res)
	})
}

// MonitorResponseTimeSec アクティビティーモニター(レスポンスタイム)取得
func (api *SimpleMonitorAPI) MonitorResponseTimeSec(id int64, body *sacloud.ResourceMonitorRequest) (*sacloud.MonitorValues, error) {
	var (
		method = "GET"
		uri    = fmt.Sprintf("%s/%d/activity/responsetimesec/monitor", api.getResourceURL(), id)
	)
	res := &sacloud.ResourceMonitorResponse{}
	err := api.baseAPI.request(method, uri, body, res)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}
