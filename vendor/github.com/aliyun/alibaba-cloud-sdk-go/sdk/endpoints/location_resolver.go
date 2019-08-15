/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package endpoints

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
)

const (
	// EndpointCacheExpireTime ...
	EndpointCacheExpireTime = 3600 //Seconds
)

// Cache caches endpoint for specific product and region
type Cache struct {
	sync.RWMutex
	cache map[string]interface{}
}

// Get ...
func (c *Cache) Get(k string) (v interface{}) {
	c.RLock()
	v = c.cache[k]
	c.RUnlock()
	return
}

// Set ...
func (c *Cache) Set(k string, v interface{}) {
	c.Lock()
	c.cache[k] = v
	c.Unlock()
}

var lastClearTimePerProduct = &Cache{cache: make(map[string]interface{})}
var endpointCache = &Cache{cache: make(map[string]interface{})}

// LocationResolver ...
type LocationResolver struct {
}

func (resolver *LocationResolver) GetName() (name string) {
	name = "location resolver"
	return
}

// TryResolve resolves endpoint giving product and region
func (resolver *LocationResolver) TryResolve(param *ResolveParam) (endpoint string, support bool, err error) {
	if len(param.LocationProduct) <= 0 {
		support = false
		return
	}

	//get from cache
	cacheKey := param.Product + "#" + param.RegionId
	var ok bool
	endpoint, ok = endpointCache.Get(cacheKey).(string)

	if ok && len(endpoint) > 0 && !CheckCacheIsExpire(cacheKey) {
		support = true
		return
	}

	//get from remote
	getEndpointRequest := requests.NewCommonRequest()

	getEndpointRequest.Product = "Location"
	getEndpointRequest.Version = "2015-06-12"
	getEndpointRequest.ApiName = "DescribeEndpoints"
	getEndpointRequest.Domain = "location-readonly.aliyuncs.com"
	getEndpointRequest.Method = "GET"
	getEndpointRequest.Scheme = requests.HTTPS

	getEndpointRequest.QueryParams["Id"] = param.RegionId
	getEndpointRequest.QueryParams["ServiceCode"] = param.LocationProduct
	if len(param.LocationEndpointType) > 0 {
		getEndpointRequest.QueryParams["Type"] = param.LocationEndpointType
	} else {
		getEndpointRequest.QueryParams["Type"] = "openAPI"
	}

	response, err := param.CommonApi(getEndpointRequest)
	if err != nil {
		support = false
		return
	}

	if !response.IsSuccess() {
		support = false
		return
	}

	var getEndpointResponse GetEndpointResponse
	err = json.Unmarshal([]byte(response.GetHttpContentString()), &getEndpointResponse)
	if err != nil {
		support = false
		return
	}

	if !getEndpointResponse.Success || getEndpointResponse.Endpoints == nil {
		support = false
		return
	}
	if len(getEndpointResponse.Endpoints.Endpoint) <= 0 {
		support = false
		return
	}
	if len(getEndpointResponse.Endpoints.Endpoint[0].Endpoint) > 0 {
		endpoint = getEndpointResponse.Endpoints.Endpoint[0].Endpoint
		endpointCache.Set(cacheKey, endpoint)
		lastClearTimePerProduct.Set(cacheKey, time.Now().Unix())
		support = true
		return
	}

	support = false
	return
}

// CheckCacheIsExpire ...
func CheckCacheIsExpire(cacheKey string) bool {
	lastClearTime, ok := lastClearTimePerProduct.Get(cacheKey).(int64)
	if !ok {
		return true
	}

	if lastClearTime <= 0 {
		lastClearTime = time.Now().Unix()
		lastClearTimePerProduct.Set(cacheKey, lastClearTime)
	}

	now := time.Now().Unix()
	elapsedTime := now - lastClearTime
	if elapsedTime > EndpointCacheExpireTime {
		return true
	}

	return false
}

// GetEndpointResponse ...
type GetEndpointResponse struct {
	Endpoints *EndpointsObj
	RequestId string
	Success   bool
}

// EndpointsObj ...
type EndpointsObj struct {
	Endpoint []EndpointObj
}

// EndpointObj ...
type EndpointObj struct {
	// Protocols   map[string]string
	Type        string
	Namespace   string
	Id          string
	SerivceCode string
	Endpoint    string
}
