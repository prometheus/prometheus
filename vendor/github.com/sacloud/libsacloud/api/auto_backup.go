package api

import (
	"encoding/json"
	//	"strings"
	"github.com/sacloud/libsacloud/sacloud"
)

// SearchAutoBackupResponse 自動バックアップ 検索レスポンス
type SearchAutoBackupResponse struct {
	// Total 総件数
	Total int `json:",omitempty"`
	// From ページング開始位置
	From int `json:",omitempty"`
	// Count 件数
	Count int `json:",omitempty"`
	// CommonServiceAutoBackupItems 自動バックアップ リスト
	CommonServiceAutoBackupItems []sacloud.AutoBackup `json:"CommonServiceItems,omitempty"`
}

type autoBackupRequest struct {
	CommonServiceAutoBackupItem *sacloud.AutoBackup    `json:"CommonServiceItem,omitempty"`
	From                        int                    `json:",omitempty"`
	Count                       int                    `json:",omitempty"`
	Sort                        []string               `json:",omitempty"`
	Filter                      map[string]interface{} `json:",omitempty"`
	Exclude                     []string               `json:",omitempty"`
	Include                     []string               `json:",omitempty"`
}

type autoBackupResponse struct {
	*sacloud.ResultFlagValue
	*sacloud.AutoBackup `json:"CommonServiceItem,omitempty"`
}

// AutoBackupAPI 自動バックアップAPI
type AutoBackupAPI struct {
	*baseAPI
}

// NewAutoBackupAPI 自動バックアップAPI作成
func NewAutoBackupAPI(client *Client) *AutoBackupAPI {
	return &AutoBackupAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "commonserviceitem"
			},
			FuncBaseSearchCondition: func() *sacloud.Request {
				res := &sacloud.Request{}
				res.AddFilter("Provider.Class", "autobackup")
				return res
			},
		},
	}
}

// Find 検索
func (api *AutoBackupAPI) Find() (*SearchAutoBackupResponse, error) {

	data, err := api.client.newRequest("GET", api.getResourceURL(), api.getSearchState())
	if err != nil {
		return nil, err
	}
	var res SearchAutoBackupResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (api *AutoBackupAPI) request(f func(*autoBackupResponse) error) (*sacloud.AutoBackup, error) {
	res := &autoBackupResponse{}
	err := f(res)
	if err != nil {
		return nil, err
	}
	return res.AutoBackup, nil
}

func (api *AutoBackupAPI) createRequest(value *sacloud.AutoBackup) *autoBackupResponse {
	return &autoBackupResponse{AutoBackup: value}
}

// New 新規作成用パラメーター作成
func (api *AutoBackupAPI) New(name string, diskID int64) *sacloud.AutoBackup {
	return sacloud.CreateNewAutoBackup(name, diskID)
}

// Create 新規作成
func (api *AutoBackupAPI) Create(value *sacloud.AutoBackup) (*sacloud.AutoBackup, error) {
	return api.request(func(res *autoBackupResponse) error {
		return api.create(api.createRequest(value), res)
	})
}

// Read 読み取り
func (api *AutoBackupAPI) Read(id int64) (*sacloud.AutoBackup, error) {
	return api.request(func(res *autoBackupResponse) error {
		return api.read(id, nil, res)
	})
}

// Update 更新
func (api *AutoBackupAPI) Update(id int64, value *sacloud.AutoBackup) (*sacloud.AutoBackup, error) {
	return api.request(func(res *autoBackupResponse) error {
		return api.update(id, api.createRequest(value), res)
	})
}

// Delete 削除
func (api *AutoBackupAPI) Delete(id int64) (*sacloud.AutoBackup, error) {
	return api.request(func(res *autoBackupResponse) error {
		return api.delete(id, nil, res)
	})
}
