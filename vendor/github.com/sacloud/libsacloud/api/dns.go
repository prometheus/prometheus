package api

import (
	"encoding/json"
	"github.com/sacloud/libsacloud/sacloud"
	"strings"
)

//HACK: さくらのAPI側仕様: CommonServiceItemsの内容によってJSONフォーマットが異なるため
//      DNS/GSLB/シンプル監視それぞれでリクエスト/レスポンスデータ型を定義する。

// SearchDNSResponse DNS検索レスポンス
type SearchDNSResponse struct {
	// Total 総件数
	Total int `json:",omitempty"`
	// From ページング開始位置
	From int `json:",omitempty"`
	// Count 件数
	Count int `json:",omitempty"`
	// CommonServiceDNSItems DNSリスト
	CommonServiceDNSItems []sacloud.DNS `json:"CommonServiceItems,omitempty"`
}
type dnsRequest struct {
	CommonServiceDNSItem *sacloud.DNS           `json:"CommonServiceItem,omitempty"`
	From                 int                    `json:",omitempty"`
	Count                int                    `json:",omitempty"`
	Sort                 []string               `json:",omitempty"`
	Filter               map[string]interface{} `json:",omitempty"`
	Exclude              []string               `json:",omitempty"`
	Include              []string               `json:",omitempty"`
}
type dnsResponse struct {
	*sacloud.ResultFlagValue
	*sacloud.DNS `json:"CommonServiceItem,omitempty"`
}

// DNSAPI DNS API
type DNSAPI struct {
	*baseAPI
}

// NewDNSAPI DNS API作成
func NewDNSAPI(client *Client) *DNSAPI {
	return &DNSAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "commonserviceitem"
			},
			FuncBaseSearchCondition: func() *sacloud.Request {
				res := &sacloud.Request{}
				res.AddFilter("Provider.Class", "dns")
				return res
			},
		},
	}
}

// Find 検索
func (api *DNSAPI) Find() (*SearchDNSResponse, error) {

	data, err := api.client.newRequest("GET", api.getResourceURL(), api.getSearchState())
	if err != nil {
		return nil, err
	}
	var res SearchDNSResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (api *DNSAPI) request(f func(*dnsResponse) error) (*sacloud.DNS, error) {
	res := &dnsResponse{}
	err := f(res)
	if err != nil {
		return nil, err
	}
	return res.DNS, nil
}

func (api *DNSAPI) createRequest(value *sacloud.DNS) *dnsRequest {
	req := &dnsRequest{}
	req.CommonServiceDNSItem = value
	return req
}

// Create 新規作成
func (api *DNSAPI) Create(value *sacloud.DNS) (*sacloud.DNS, error) {
	return api.request(func(res *dnsResponse) error {
		return api.create(api.createRequest(value), res)
	})
}

// New 新規作成用パラメーター作成
func (api *DNSAPI) New(zoneName string) *sacloud.DNS {
	return sacloud.CreateNewDNS(zoneName)
}

// Read 読み取り
func (api *DNSAPI) Read(id int64) (*sacloud.DNS, error) {
	return api.request(func(res *dnsResponse) error {
		return api.read(id, nil, res)
	})
}

// Update 更新
func (api *DNSAPI) Update(id int64, value *sacloud.DNS) (*sacloud.DNS, error) {
	return api.request(func(res *dnsResponse) error {
		return api.update(id, api.createRequest(value), res)
	})
}

// Delete 削除
func (api *DNSAPI) Delete(id int64) (*sacloud.DNS, error) {
	return api.request(func(res *dnsResponse) error {
		return api.delete(id, nil, res)
	})
}

// SetupDNSRecord DNSレコード作成
func (api *DNSAPI) SetupDNSRecord(zoneName string, hostName string, ip string) ([]string, error) {

	dnsItem, err := api.findOrCreateBy(zoneName)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(hostName, zoneName) {
		hostName = strings.Replace(hostName, zoneName, "", -1)
	}

	dnsItem.Settings.DNS.AddDNSRecordSet(hostName, ip)

	res, err := api.updateDNSRecord(dnsItem)
	if err != nil {
		return nil, err
	}

	if dnsItem.ID == sacloud.EmptyID {
		return res.Status.NS, nil
	}

	return nil, nil

}

// DeleteDNSRecord DNSレコード削除
func (api *DNSAPI) DeleteDNSRecord(zoneName string, hostName string, ip string) error {
	dnsItem, err := api.findOrCreateBy(zoneName)
	if err != nil {
		return err
	}
	dnsItem.Settings.DNS.DeleteDNSRecordSet(hostName, ip)

	if dnsItem.HasDNSRecord() {
		_, err = api.updateDNSRecord(dnsItem)
		if err != nil {
			return err
		}

	} else {
		_, err = api.Delete(dnsItem.ID)
		if err != nil {
			return err
		}

	}
	return nil
}

func (api *DNSAPI) findOrCreateBy(zoneName string) (*sacloud.DNS, error) {

	res, err := api.Reset().WithNameLike(zoneName).Limit(1).Find()
	if err != nil {
		return nil, err
	}

	//すでに登録されている場合
	var dnsItem *sacloud.DNS
	if res.Count > 0 {
		dnsItem = &res.CommonServiceDNSItems[0]
	} else {
		dnsItem = sacloud.CreateNewDNS(zoneName)
	}

	return dnsItem, nil
}

func (api *DNSAPI) updateDNSRecord(dnsItem *sacloud.DNS) (*sacloud.DNS, error) {

	var item *sacloud.DNS
	var err error

	if dnsItem.ID == sacloud.EmptyID {
		item, err = api.Create(dnsItem)
	} else {
		item, err = api.Update(dnsItem.ID, dnsItem)
	}

	if err != nil {
		return nil, err
	}

	return item, nil
}
