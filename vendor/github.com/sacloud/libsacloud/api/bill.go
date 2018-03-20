package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
	"io"
	"strings"
	"time"
)

// BillAPI 請求情報API
type BillAPI struct {
	*baseAPI
}

// NewBillAPI 請求情報API作成
func NewBillAPI(client *Client) *BillAPI {
	return &BillAPI{
		&baseAPI{
			client:        client,
			apiRootSuffix: sakuraBillingAPIRootSuffix,
			FuncGetResourceURL: func() string {
				return "bill"
			},
		},
	}
}

// BillResponse 請求情報レスポンス
type BillResponse struct {
	*sacloud.ResultFlagValue
	// Count 件数
	Count int `json:",omitempty"`
	// ResponsedAt 応答日時
	ResponsedAt *time.Time `json:",omitempty"`
	// Bills 請求情報 リスト
	Bills []*sacloud.Bill
}

// BillDetailResponse 請求明細レスポンス
type BillDetailResponse struct {
	*sacloud.ResultFlagValue
	// Count 件数
	Count int `json:",omitempty"`
	// ResponsedAt 応答日時
	ResponsedAt *time.Time `json:",omitempty"`
	// BillDetails 請求明細 リスト
	BillDetails []*sacloud.BillDetail
}

// BillDetailCSVResponse 請求明細CSVレスポンス
type BillDetailCSVResponse struct {
	*sacloud.ResultFlagValue
	// Count 件数
	Count int `json:",omitempty"`
	// ResponsedAt 応答日時
	ResponsedAt *time.Time `json:",omitempty"`
	// Filename ファイル名
	Filename string `json:",omitempty"`
	// RawBody ボディ(未加工)
	RawBody string `json:"Body,omitempty"`
	// HeaderRow ヘッダ行
	HeaderRow []string
	// BodyRows ボディ(各行/各列での配列)
	BodyRows [][]string
}

func (res *BillDetailCSVResponse) buildCSVBody() {

	if res == nil || res.RawBody == "" {
		return
	}

	//CSV分割(先頭行/それ以降)、
	reader := csv.NewReader(strings.NewReader(res.RawBody))
	reader.LazyQuotes = true

	isFirst := true
	res.BodyRows = [][]string{}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}

		if isFirst {
			res.HeaderRow = record
			isFirst = false
		} else {
			res.BodyRows = append(res.BodyRows, record)
		}
	}
}

// ByContract アカウントIDごとの請求取得
func (api *BillAPI) ByContract(accountID int64) (*BillResponse, error) {

	uri := fmt.Sprintf("%s/by-contract/%d", api.getResourceURL(), accountID)
	return api.getContract(uri)
}

// ByContractYear 年指定での請求取得
func (api *BillAPI) ByContractYear(accountID int64, year int) (*BillResponse, error) {
	uri := fmt.Sprintf("%s/by-contract/%d/%d", api.getResourceURL(), accountID, year)
	return api.getContract(uri)
}

// ByContractYearMonth 年月指定での請求指定
func (api *BillAPI) ByContractYearMonth(accountID int64, year int, month int) (*BillResponse, error) {
	uri := fmt.Sprintf("%s/by-contract/%d/%d/%d", api.getResourceURL(), accountID, year, month)
	return api.getContract(uri)
}

// Read 読み取り
func (api *BillAPI) Read(billNo int64) (*BillResponse, error) {
	uri := fmt.Sprintf("%s/id/%d/", api.getResourceURL(), billNo)
	return api.getContract(uri)

}

func (api *BillAPI) getContract(uri string) (*BillResponse, error) {

	data, err := api.client.newRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}
	var res BillResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil

}

// GetDetail 請求明細取得
func (api *BillAPI) GetDetail(memberCD string, billNo int64) (*BillDetailResponse, error) {

	oldFunc := api.FuncGetResourceURL
	defer func() { api.FuncGetResourceURL = oldFunc }()
	api.FuncGetResourceURL = func() string {
		return "billdetail"
	}

	uri := fmt.Sprintf("%s/%s/%d", api.getResourceURL(), memberCD, billNo)
	data, err := api.client.newRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}
	var res BillDetailResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil

}

// GetDetailCSV 請求明細CSV取得
func (api *BillAPI) GetDetailCSV(memberCD string, billNo int64) (*BillDetailCSVResponse, error) {

	oldFunc := api.FuncGetResourceURL
	defer func() { api.FuncGetResourceURL = oldFunc }()
	api.FuncGetResourceURL = func() string {
		return "billdetail"
	}

	uri := fmt.Sprintf("%s/%s/%d/csv", api.getResourceURL(), memberCD, billNo)
	data, err := api.client.newRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}
	var res BillDetailCSVResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	// build HeaderRow and BodyRows from RawBody
	res.buildCSVBody()

	return &res, nil

}
