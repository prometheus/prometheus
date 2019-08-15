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

package requests

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

type RoaRequest struct {
	*baseRequest
	pathPattern string
	PathParams  map[string]string
}

func (*RoaRequest) GetStyle() string {
	return ROA
}

func (request *RoaRequest) GetBodyReader() io.Reader {
	if request.FormParams != nil && len(request.FormParams) > 0 {
		formString := utils.GetUrlFormedMap(request.FormParams)
		return strings.NewReader(formString)
	} else if len(request.Content) > 0 {
		return bytes.NewReader(request.Content)
	} else {
		return nil
	}
}

// for sign method, need not url encoded
func (request *RoaRequest) BuildQueries() string {
	return request.buildQueries()
}

func (request *RoaRequest) buildPath() string {
	path := request.pathPattern
	for key, value := range request.PathParams {
		path = strings.Replace(path, "["+key+"]", value, 1)
	}
	return path
}

func (request *RoaRequest) buildQueries() string {
	// replace path params with value
	path := request.buildPath()
	queryParams := request.QueryParams
	// sort QueryParams by key
	var queryKeys []string
	for key := range queryParams {
		queryKeys = append(queryKeys, key)
	}
	sort.Strings(queryKeys)

	// append urlBuilder
	urlBuilder := bytes.Buffer{}
	urlBuilder.WriteString(path)
	if len(queryKeys) > 0 {
		urlBuilder.WriteString("?")
	}
	for i := 0; i < len(queryKeys); i++ {
		queryKey := queryKeys[i]
		urlBuilder.WriteString(queryKey)
		if value := queryParams[queryKey]; len(value) > 0 {
			urlBuilder.WriteString("=")
			urlBuilder.WriteString(value)
		}
		if i < len(queryKeys)-1 {
			urlBuilder.WriteString("&")
		}
	}
	result := urlBuilder.String()
	result = popStandardUrlencode(result)
	return result
}

func (request *RoaRequest) buildQueryString() string {
	queryParams := request.QueryParams
	// sort QueryParams by key
	q := url.Values{}
	for key, value := range queryParams {
		q.Add(key, value)
	}
	return q.Encode()
}

func popStandardUrlencode(stringToSign string) (result string) {
	result = strings.Replace(stringToSign, "+", "%20", -1)
	result = strings.Replace(result, "*", "%2A", -1)
	result = strings.Replace(result, "%7E", "~", -1)
	return
}

func (request *RoaRequest) BuildUrl() string {
	// for network trans, need url encoded
	scheme := strings.ToLower(request.Scheme)
	domain := request.Domain
	port := request.Port
	path := request.buildPath()
	url := fmt.Sprintf("%s://%s:%s%s", scheme, domain, port, path)
	querystring := request.buildQueryString()
	if len(querystring) > 0 {
		url = fmt.Sprintf("%s?%s", url, querystring)
	}
	return url
}

func (request *RoaRequest) addPathParam(key, value string) {
	request.PathParams[key] = value
}

func (request *RoaRequest) InitWithApiInfo(product, version, action, uriPattern, serviceCode, endpointType string) {
	request.baseRequest = defaultBaseRequest()
	request.PathParams = make(map[string]string)
	request.Headers["x-acs-version"] = version
	request.pathPattern = uriPattern
	request.locationServiceCode = serviceCode
	request.locationEndpointType = endpointType
	request.product = product
	//request.version = version
	request.actionName = action
}

func (request *RoaRequest) initWithCommonRequest(commonRequest *CommonRequest) {
	request.baseRequest = commonRequest.baseRequest
	request.PathParams = commonRequest.PathParams
	request.product = commonRequest.Product
	//request.version = commonRequest.Version
	request.Headers["x-acs-version"] = commonRequest.Version
	request.actionName = commonRequest.ApiName
	request.pathPattern = commonRequest.PathPattern
	request.locationServiceCode = commonRequest.ServiceCode
	request.locationEndpointType = ""
}
