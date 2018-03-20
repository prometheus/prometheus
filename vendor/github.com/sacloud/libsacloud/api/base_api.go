package api

import (
	"encoding/json"
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
	"net/url"
)

type baseAPI struct {
	client                  *Client
	FuncGetResourceURL      func() string
	FuncBaseSearchCondition func() *sacloud.Request
	state                   *sacloud.Request
	apiRootSuffix           string
}

var (
	sakuraCloudAPIRootSuffix    = "api/cloud/1.1"
	sakuraBillingAPIRootSuffix  = "api/system/1.0"
	sakuraWebAccelAPIRootSuffix = "api/webaccel/1.0"
)

func (api *baseAPI) getResourceURL() string {

	suffix := api.apiRootSuffix
	//デフォルト : クラウドAPI
	if suffix == "" {
		suffix = sakuraCloudAPIRootSuffix
	}

	url := ""
	if api.FuncGetResourceURL != nil {
		url = api.FuncGetResourceURL()
	}

	if suffix == "" {
		return url
	}
	if url == "" {
		return suffix
	}

	return fmt.Sprintf("%s/%s", suffix, url)
}

func (api *baseAPI) getSearchState() *sacloud.Request {
	if api.state == nil {
		api.reset()
	}
	return api.state
}
func (api *baseAPI) sortBy(key string, reverse bool) *baseAPI {
	return api.setStateValue(func(state *sacloud.Request) {
		if state.Sort == nil {
			state.Sort = []string{}
		}

		col := key
		if reverse {
			col = "-" + col
		}
		state.Sort = append(state.Sort, col)

	})

}

func (api *baseAPI) reset() *baseAPI {
	if api.FuncBaseSearchCondition == nil {
		api.state = &sacloud.Request{}
	} else {
		api.state = api.FuncBaseSearchCondition()
	}
	return api
}

func (api *baseAPI) setStateValue(setFunc func(*sacloud.Request)) *baseAPI {
	state := api.getSearchState()
	setFunc(state)
	return api

}

func (api *baseAPI) offset(offset int) *baseAPI {
	return api.setStateValue(func(state *sacloud.Request) {
		state.From = offset
	})
}

func (api *baseAPI) limit(limit int) *baseAPI {
	return api.setStateValue(func(state *sacloud.Request) {
		state.Count = limit
	})
}

func (api *baseAPI) include(key string) *baseAPI {
	return api.setStateValue(func(state *sacloud.Request) {
		if state.Include == nil {
			state.Include = []string{}
		}
		state.Include = append(state.Include, key)
	})
}

func (api *baseAPI) exclude(key string) *baseAPI {
	return api.setStateValue(func(state *sacloud.Request) {
		if state.Exclude == nil {
			state.Exclude = []string{}
		}
		state.Exclude = append(state.Exclude, key)
	})
}

func (api *baseAPI) filterBy(key string, value interface{}, multiple bool) *baseAPI {
	return api.setStateValue(func(state *sacloud.Request) {

		//HACK さくらのクラウド側でqueryStringでの+エスケープに対応していないため、
		// %20にエスケープされるurl.Pathを利用する。
		// http://qiita.com/shibukawa/items/c0730092371c0e243f62
		if strValue, ok := value.(string); ok {
			u := &url.URL{Path: strValue}
			value = u.String()
		}

		if state.Filter == nil {
			state.Filter = map[string]interface{}{}
		}
		if multiple {
			if state.Filter[key] == nil {
				state.Filter[key] = []interface{}{}
			}

			state.Filter[key] = append(state.Filter[key].([]interface{}), value)
		} else {
			// どちらもstring型の場合はスペース区切りで繋げる
			if f, ok := state.Filter[key]; ok {
				if s, ok := f.(string); ok && s != "" {
					if v, ok := value.(string); ok {
						state.Filter[key] = fmt.Sprintf("%s %s", s, v)
						return
					}
				}
			}
			state.Filter[key] = value

		}
	})
}

func (api *baseAPI) withNameLike(name string) *baseAPI {
	return api.filterBy("Name", name, false)
}

func (api *baseAPI) withTag(tag string) *baseAPI {
	return api.filterBy("Tags.Name", tag, false)
}

func (api *baseAPI) withTags(tags []string) *baseAPI {
	return api.filterBy("Tags.Name", tags, false)
}

func (api *baseAPI) sortByName(reverse bool) *baseAPI {
	return api.sortBy("Name", reverse)
}

func (api *baseAPI) Find() (*sacloud.SearchResponse, error) {

	data, err := api.client.newRequest("GET", api.getResourceURL(), api.getSearchState())
	if err != nil {
		return nil, err
	}
	var res sacloud.SearchResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (api *baseAPI) request(method string, uri string, body interface{}, res interface{}) error {
	data, err := api.client.newRequest(method, uri, body)
	if err != nil {
		return err
	}

	if res != nil {
		if err := json.Unmarshal(data, &res); err != nil {
			return err
		}
	}
	return nil
}

func (api *baseAPI) create(body interface{}, res interface{}) error {
	var (
		method = "POST"
		uri    = api.getResourceURL()
	)

	return api.request(method, uri, body, res)
}

func (api *baseAPI) read(id int64, body interface{}, res interface{}) error {
	var (
		method = "GET"
		uri    = fmt.Sprintf("%s/%d", api.getResourceURL(), id)
	)

	return api.request(method, uri, body, res)
}

func (api *baseAPI) update(id int64, body interface{}, res interface{}) error {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d", api.getResourceURL(), id)
	)
	return api.request(method, uri, body, res)
}

func (api *baseAPI) delete(id int64, body interface{}, res interface{}) error {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("%s/%d", api.getResourceURL(), id)
	)
	return api.request(method, uri, body, res)
}

func (api *baseAPI) modify(method string, uri string, body interface{}) (bool, error) {
	res := &sacloud.ResultFlagValue{}
	err := api.request(method, uri, body, res)
	if err != nil {
		return false, err
	}
	return res.IsOk, nil
}

func (api *baseAPI) action(method string, uri string, body interface{}, res interface{}) (bool, error) {
	err := api.request(method, uri, body, res)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (api *baseAPI) monitor(id int64, body *sacloud.ResourceMonitorRequest) (*sacloud.MonitorValues, error) {
	var (
		method = "GET"
		uri    = fmt.Sprintf("%s/%d/monitor", api.getResourceURL(), id)
	)
	res := &sacloud.ResourceMonitorResponse{}
	err := api.request(method, uri, body, res)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (api *baseAPI) applianceMonitorBy(id int64, target string, nicIndex int, body *sacloud.ResourceMonitorRequest) (*sacloud.MonitorValues, error) {
	var (
		method = "GET"
		uri    = fmt.Sprintf("%s/%d/%s/%d/monitor", api.getResourceURL(), id, target, nicIndex)
	)
	if nicIndex == 0 {
		uri = fmt.Sprintf("%s/%d/%s/monitor", api.getResourceURL(), id, target)
	}

	res := &sacloud.ResourceMonitorResponse{}
	err := api.request(method, uri, body, res)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (api *baseAPI) NewResourceMonitorRequest() *sacloud.ResourceMonitorRequest {
	return &sacloud.ResourceMonitorRequest{}
}
