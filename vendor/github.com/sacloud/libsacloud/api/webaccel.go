package api

import (
	"encoding/json"
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
	"strings"
)

// WebAccelAPI ウェブアクセラレータAPI
type WebAccelAPI struct {
	*baseAPI
}

// NewWebAccelAPI ウェブアクセラレータAPI
func NewWebAccelAPI(client *Client) *WebAccelAPI {
	return &WebAccelAPI{
		&baseAPI{
			client:        client,
			apiRootSuffix: sakuraWebAccelAPIRootSuffix,
			FuncGetResourceURL: func() string {
				return ""
			},
		},
	}
}

// WebAccelDeleteCacheResponse ウェブアクセラレータ キャッシュ削除レスポンス
type WebAccelDeleteCacheResponse struct {
	*sacloud.ResultFlagValue
	Results []*sacloud.DeleteCacheResult
}

// Read サイト情報取得
func (api *WebAccelAPI) Read(id string) (*sacloud.WebAccelSite, error) {

	uri := fmt.Sprintf("%s/site/%s", api.getResourceURL(), id)

	data, err := api.client.newRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Site sacloud.WebAccelSite
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res.Site, nil
}

// ReadCertificate 証明書 参照
func (api *WebAccelAPI) ReadCertificate(id string) (*sacloud.WebAccelCertResponseBody, error) {
	uri := fmt.Sprintf("%s/site/%s/certificate", api.getResourceURL(), id)

	data, err := api.client.newRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	var res sacloud.WebAccelCertResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return res.Certificate, nil
}

// UpdateCertificate 証明書 更新
func (api *WebAccelAPI) UpdateCertificate(id string, request *sacloud.WebAccelCertRequest) (*sacloud.WebAccelCertResponse, error) {
	uri := fmt.Sprintf("%s/site/%s/certificate", api.getResourceURL(), id)

	if request.CertificateChain != "" {
		request.CertificateChain = strings.TrimRight(request.CertificateChain, "\n")
	}
	if request.Key != "" {
		request.Key = strings.TrimRight(request.Key, "\n")
	}

	data, err := api.client.newRequest("PUT", uri, map[string]interface{}{
		"Certificate": request,
	})
	if err != nil {
		return nil, err
	}

	var res sacloud.WebAccelCertResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// DeleteCache キャッシュ削除
func (api *WebAccelAPI) DeleteCache(urls ...string) (*WebAccelDeleteCacheResponse, error) {

	type request struct {
		// URL
		URL []string
	}

	uri := fmt.Sprintf("%s/deletecache", api.getResourceURL())

	data, err := api.client.newRequest("POST", uri, &request{URL: urls})
	if err != nil {
		return nil, err
	}

	var res WebAccelDeleteCacheResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
