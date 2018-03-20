package api

import (
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
	"time"
)

// InternetAPI ルーターAPI
type InternetAPI struct {
	*baseAPI
}

// NewInternetAPI ルーターAPI作成
func NewInternetAPI(client *Client) *InternetAPI {
	return &InternetAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "internet"
			},
		},
	}
}

// UpdateBandWidth 帯域幅更新
func (api *InternetAPI) UpdateBandWidth(id int64, bandWidth int) (*sacloud.Internet, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/bandwidth", api.getResourceURL(), id)
		body   = &sacloud.Request{}
	)
	body.Internet = &sacloud.Internet{BandWidthMbps: bandWidth}

	return api.request(func(res *sacloud.Response) error {
		return api.baseAPI.request(method, uri, body, res)
	})
}

// AddSubnet IPアドレスブロックの追加割り当て
func (api *InternetAPI) AddSubnet(id int64, nwMaskLen int, nextHop string) (*sacloud.Subnet, error) {
	var (
		method = "POST"
		uri    = fmt.Sprintf("%s/%d/subnet", api.getResourceURL(), id)
	)
	body := &map[string]interface{}{
		"NetworkMaskLen": nwMaskLen,
		"NextHop":        nextHop,
	}

	res := &sacloud.Response{}
	err := api.baseAPI.request(method, uri, body, res)
	if err != nil {
		return nil, err
	}
	return res.Subnet, nil
}

// UpdateSubnet IPアドレスブロックの更新
func (api *InternetAPI) UpdateSubnet(id int64, subnetID int64, nextHop string) (*sacloud.Subnet, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/subnet/%d", api.getResourceURL(), id, subnetID)
	)
	body := &map[string]interface{}{
		"NextHop": nextHop,
	}

	res := &sacloud.Response{}
	err := api.baseAPI.request(method, uri, body, res)
	if err != nil {
		return nil, err
	}
	return res.Subnet, nil
}

// DeleteSubnet IPアドレスブロックの削除
func (api *InternetAPI) DeleteSubnet(id int64, subnetID int64) (*sacloud.ResultFlagValue, error) {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("%s/%d/subnet/%d", api.getResourceURL(), id, subnetID)
	)

	res := &sacloud.ResultFlagValue{}
	err := api.baseAPI.request(method, uri, nil, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// EnableIPv6 IPv6有効化
func (api *InternetAPI) EnableIPv6(id int64) (*sacloud.IPv6Net, error) {
	var (
		method = "POST"
		uri    = fmt.Sprintf("%s/%d/ipv6net", api.getResourceURL(), id)
	)

	res := &sacloud.Response{}
	err := api.baseAPI.request(method, uri, nil, res)
	if err != nil {
		return nil, err
	}
	return res.IPv6Net, nil
}

// DisableIPv6 IPv6無効化
func (api *InternetAPI) DisableIPv6(id int64, ipv6NetID int64) (bool, error) {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("%s/%d/ipv6net/%d", api.getResourceURL(), id, ipv6NetID)
	)

	res := &sacloud.Response{}
	err := api.baseAPI.request(method, uri, nil, res)
	if err != nil {
		return false, err
	}
	return true, nil
}

// SleepWhileCreating 作成完了まで待機(リトライ10)
func (api *InternetAPI) SleepWhileCreating(id int64, timeout time.Duration) error {
	handler := waitingForReadFunc(func() (interface{}, error) {
		return api.Read(id)
	}, 10) // 作成直後はReadが404を返すことがあるためリトライ
	return blockingPoll(handler, timeout)

}

// RetrySleepWhileCreating 作成完了まで待機 作成直後はReadが404を返すことがあるためmaxRetryまでリトライする
func (api *InternetAPI) RetrySleepWhileCreating(id int64, timeout time.Duration, maxRetry int) error {
	handler := waitingForReadFunc(func() (interface{}, error) {
		return api.Read(id)
	}, maxRetry)
	return blockingPoll(handler, timeout)

}

// Monitor アクティビティーモニター取得
func (api *InternetAPI) Monitor(id int64, body *sacloud.ResourceMonitorRequest) (*sacloud.MonitorValues, error) {
	return api.baseAPI.monitor(id, body)
}
