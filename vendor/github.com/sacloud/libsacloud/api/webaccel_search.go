package api

import (
	"encoding/json"
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
	"net/url"
	"strings"
)

// Reset 検索条件のリセット
func (api *WebAccelAPI) Reset() *WebAccelAPI {
	api.SetEmpty()
	return api
}

// SetEmpty 検索条件のリセット
func (api *WebAccelAPI) SetEmpty() {
	api.reset()
}

// FilterBy 指定キーでのフィルター
func (api *WebAccelAPI) FilterBy(key string, value interface{}) *WebAccelAPI {
	api.filterBy(key, value, false)
	return api
}

// WithNameLike 名称条件
func (api *WebAccelAPI) WithNameLike(name string) *WebAccelAPI {
	return api.FilterBy("Name", name)
}

// SetFilterBy 指定キーでのフィルター
func (api *WebAccelAPI) SetFilterBy(key string, value interface{}) {
	api.filterBy(key, value, false)
}

// SetNameLike 名称条件
func (api *WebAccelAPI) SetNameLike(name string) {
	api.FilterBy("Name", name)
}

// Find サイト一覧取得
func (api *WebAccelAPI) Find() (*sacloud.SearchResponse, error) {

	uri := fmt.Sprintf("%s/site", api.getResourceURL())

	data, err := api.client.newRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	var res sacloud.SearchResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	// handle filter(API側がFilterに対応していないためここでフィルタリング)
	for key, filter := range api.getSearchState().Filter {
		if key != "Name" {
			continue
		}
		strNames, ok := filter.(string)
		if !ok {
			continue
		}

		names := strings.Split(strNames, " ")
		filtered := []sacloud.WebAccelSite{}
		for _, site := range res.WebAccelSites {
			for _, name := range names {

				u, _ := url.Parse(name)

				if strings.Contains(site.Name, u.Path) {
					filtered = append(filtered, site)
					break
				}
			}
		}
		res.WebAccelSites = filtered
	}

	return &res, nil
}
