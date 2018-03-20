package api

import (
	"encoding/json"
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
	"time"
)

//HACK: さくらのAPI側仕様: Applianceの内容によってJSONフォーマットが異なるため
//      ロードバランサ/VPCルータそれぞれでリクエスト/レスポンスデータ型を定義する。

// SearchLoadBalancerResponse ロードバランサー検索レスポンス
type SearchLoadBalancerResponse struct {
	// Total 総件数
	Total int `json:",omitempty"`
	// From ページング開始位置
	From int `json:",omitempty"`
	// Count 件数
	Count int `json:",omitempty"`
	// LoadBalancers ロードバランサー リスト
	LoadBalancers []sacloud.LoadBalancer `json:"Appliances,omitempty"`
}

type loadBalancerRequest struct {
	LoadBalancer *sacloud.LoadBalancer  `json:"Appliance,omitempty"`
	From         int                    `json:",omitempty"`
	Count        int                    `json:",omitempty"`
	Sort         []string               `json:",omitempty"`
	Filter       map[string]interface{} `json:",omitempty"`
	Exclude      []string               `json:",omitempty"`
	Include      []string               `json:",omitempty"`
}

type loadBalancerResponse struct {
	*sacloud.ResultFlagValue
	*sacloud.LoadBalancer `json:"Appliance,omitempty"`
	Success               interface{} `json:",omitempty"` //HACK: さくらのAPI側仕様: 戻り値:Successがbool値へ変換できないためinterface{}
}

// LoadBalancerAPI ロードバランサーAPI
type LoadBalancerAPI struct {
	*baseAPI
}

// NewLoadBalancerAPI ロードバランサーAPI作成
func NewLoadBalancerAPI(client *Client) *LoadBalancerAPI {
	return &LoadBalancerAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "appliance"
			},
			FuncBaseSearchCondition: func() *sacloud.Request {
				res := &sacloud.Request{}
				res.AddFilter("Class", "loadbalancer")
				return res
			},
		},
	}
}

// Find 検索
func (api *LoadBalancerAPI) Find() (*SearchLoadBalancerResponse, error) {
	data, err := api.client.newRequest("GET", api.getResourceURL(), api.getSearchState())
	if err != nil {
		return nil, err
	}
	var res SearchLoadBalancerResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (api *LoadBalancerAPI) request(f func(*loadBalancerResponse) error) (*sacloud.LoadBalancer, error) {
	res := &loadBalancerResponse{}
	err := f(res)
	if err != nil {
		return nil, err
	}
	return res.LoadBalancer, nil
}

func (api *LoadBalancerAPI) createRequest(value *sacloud.LoadBalancer) *loadBalancerResponse {
	return &loadBalancerResponse{LoadBalancer: value}
}

//func (api *LoadBalancerAPI) New() *sacloud.LoadBalancer {
//	return sacloud.CreateNewLoadBalancer()
//}

// Create 新規作成
func (api *LoadBalancerAPI) Create(value *sacloud.LoadBalancer) (*sacloud.LoadBalancer, error) {
	return api.request(func(res *loadBalancerResponse) error {
		return api.create(api.createRequest(value), res)
	})
}

// Read 読み取り
func (api *LoadBalancerAPI) Read(id int64) (*sacloud.LoadBalancer, error) {
	return api.request(func(res *loadBalancerResponse) error {
		return api.read(id, nil, res)
	})
}

// Update 更新
func (api *LoadBalancerAPI) Update(id int64, value *sacloud.LoadBalancer) (*sacloud.LoadBalancer, error) {
	return api.request(func(res *loadBalancerResponse) error {
		return api.update(id, api.createRequest(value), res)
	})
}

// Delete 削除
func (api *LoadBalancerAPI) Delete(id int64) (*sacloud.LoadBalancer, error) {
	return api.request(func(res *loadBalancerResponse) error {
		return api.delete(id, nil, res)
	})
}

// Config 設定変更の反映
func (api *LoadBalancerAPI) Config(id int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/config", api.getResourceURL(), id)
	)
	return api.modify(method, uri, nil)
}

// IsUp 起動しているか判定
func (api *LoadBalancerAPI) IsUp(id int64) (bool, error) {
	lb, err := api.Read(id)
	if err != nil {
		return false, err
	}
	return lb.Instance.IsUp(), nil
}

// IsDown ダウンしているか判定
func (api *LoadBalancerAPI) IsDown(id int64) (bool, error) {
	lb, err := api.Read(id)
	if err != nil {
		return false, err
	}
	return lb.Instance.IsDown(), nil
}

// Boot 起動
func (api *LoadBalancerAPI) Boot(id int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/power", api.getResourceURL(), id)
	)
	return api.modify(method, uri, nil)
}

// Shutdown シャットダウン(graceful)
func (api *LoadBalancerAPI) Shutdown(id int64) (bool, error) {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("%s/%d/power", api.getResourceURL(), id)
	)

	return api.modify(method, uri, nil)
}

// Stop シャットダウン(force)
func (api *LoadBalancerAPI) Stop(id int64) (bool, error) {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("%s/%d/power", api.getResourceURL(), id)
	)

	return api.modify(method, uri, map[string]bool{"Force": true})
}

// RebootForce 再起動
func (api *LoadBalancerAPI) RebootForce(id int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/reset", api.getResourceURL(), id)
	)

	return api.modify(method, uri, nil)
}

// ResetForce リセット
func (api *LoadBalancerAPI) ResetForce(id int64, recycleProcess bool) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/reset", api.getResourceURL(), id)
	)

	return api.modify(method, uri, map[string]bool{"RecycleProcess": recycleProcess})
}

// SleepUntilUp 起動するまで待機
func (api *LoadBalancerAPI) SleepUntilUp(id int64, timeout time.Duration) error {
	handler := waitingForUpFunc(func() (hasUpDown, error) {
		return api.Read(id)
	}, 0)
	return blockingPoll(handler, timeout)
}

// SleepUntilDown ダウンするまで待機
func (api *LoadBalancerAPI) SleepUntilDown(id int64, timeout time.Duration) error {
	handler := waitingForDownFunc(func() (hasUpDown, error) {
		return api.Read(id)
	}, 0)
	return blockingPoll(handler, timeout)
}

// SleepWhileCopying コピー終了まで待機
func (api *LoadBalancerAPI) SleepWhileCopying(id int64, timeout time.Duration, maxRetry int) error {
	handler := waitingForAvailableFunc(func() (hasAvailable, error) {
		return api.Read(id)
	}, maxRetry)
	return blockingPoll(handler, timeout)
}

// AsyncSleepWhileCopying コピー終了まで待機(非同期)
func (api *LoadBalancerAPI) AsyncSleepWhileCopying(id int64, timeout time.Duration, maxRetry int) (chan (interface{}), chan (interface{}), chan (error)) {
	handler := waitingForAvailableFunc(func() (hasAvailable, error) {
		return api.Read(id)
	}, maxRetry)
	return poll(handler, timeout)
}

// Monitor アクティビティーモニター取得
func (api *LoadBalancerAPI) Monitor(id int64, body *sacloud.ResourceMonitorRequest) (*sacloud.MonitorValues, error) {
	return api.baseAPI.applianceMonitorBy(id, "interface", 0, body)
}
