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
	"fmt"
	"sync"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

var debug utils.Debug

func init() {
	debug = utils.Init("sdk")
}

const (
	ResolveEndpointUserGuideLink = ""
)

var once sync.Once
var resolvers []Resolver

type Resolver interface {
	TryResolve(param *ResolveParam) (endpoint string, support bool, err error)
	GetName() (name string)
}

// Resolve resolve endpoint with params
// It will resolve with each supported resolver until anyone resolved
func Resolve(param *ResolveParam) (endpoint string, err error) {
	supportedResolvers := getAllResolvers()
	var lastErr error
	for _, resolver := range supportedResolvers {
		endpoint, supported, resolveErr := resolver.TryResolve(param)
		if resolveErr != nil {
			lastErr = resolveErr
		}

		if supported {
			debug("resolve endpoint with %s\n", param)
			debug("\t%s by resolver(%s)\n", endpoint, resolver.GetName())
			return endpoint, nil
		}
	}

	// not support
	errorMsg := fmt.Sprintf(errors.CanNotResolveEndpointErrorMessage, param, ResolveEndpointUserGuideLink)
	err = errors.NewClientError(errors.CanNotResolveEndpointErrorCode, errorMsg, lastErr)
	return
}

func getAllResolvers() []Resolver {
	once.Do(func() {
		resolvers = []Resolver{
			&SimpleHostResolver{},
			&MappingResolver{},
			&LocationResolver{},
			&LocalRegionalResolver{},
			&LocalGlobalResolver{},
		}
	})
	return resolvers
}

type ResolveParam struct {
	Domain               string
	Product              string
	RegionId             string
	LocationProduct      string
	LocationEndpointType string
	CommonApi            func(request *requests.CommonRequest) (response *responses.CommonResponse, err error) `json:"-"`
}

func (param *ResolveParam) String() string {
	jsonBytes, err := json.Marshal(param)
	if err != nil {
		return fmt.Sprint("ResolveParam.String() process error:", err)
	}
	return string(jsonBytes)
}
