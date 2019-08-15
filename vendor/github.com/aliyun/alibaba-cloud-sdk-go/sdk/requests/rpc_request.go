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
	"fmt"
	"io"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

type RpcRequest struct {
	*baseRequest
}

func (request *RpcRequest) init() {
	request.baseRequest = defaultBaseRequest()
	request.Method = POST
}

func (*RpcRequest) GetStyle() string {
	return RPC
}

func (request *RpcRequest) GetBodyReader() io.Reader {
	if request.FormParams != nil && len(request.FormParams) > 0 {
		formString := utils.GetUrlFormedMap(request.FormParams)
		return strings.NewReader(formString)
	} else {
		return strings.NewReader("")
	}
}

func (request *RpcRequest) BuildQueries() string {
	request.queries = "/?" + utils.GetUrlFormedMap(request.QueryParams)
	return request.queries
}

func (request *RpcRequest) BuildUrl() string {
	url := fmt.Sprintf("%s://%s", strings.ToLower(request.Scheme), request.Domain)
	if len(request.Port) > 0 {
		url = fmt.Sprintf("%s:%s", url, request.Port)
	}
	return url + request.BuildQueries()
}

func (request *RpcRequest) GetVersion() string {
	return request.version
}

func (request *RpcRequest) GetActionName() string {
	return request.actionName
}

func (request *RpcRequest) addPathParam(key, value string) {
	panic("not support")
}

func (request *RpcRequest) InitWithApiInfo(product, version, action, serviceCode, endpointType string) {
	request.init()
	request.product = product
	request.version = version
	request.actionName = action
	request.locationServiceCode = serviceCode
	request.locationEndpointType = endpointType
}
