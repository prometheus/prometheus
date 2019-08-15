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
	"fmt"
	"strings"

	"github.com/jmespath/go-jmespath"
)

type LocalRegionalResolver struct {
}

func (resolver *LocalRegionalResolver) GetName() (name string) {
	name = "local regional resolver"
	return
}

func (resolver *LocalRegionalResolver) TryResolve(param *ResolveParam) (endpoint string, support bool, err error) {
	// get the regional endpoints configs
	regionalExpression := fmt.Sprintf("products[?code=='%s'].regional_endpoints", strings.ToLower(param.Product))
	regionalData, err := jmespath.Search(regionalExpression, getEndpointConfigData())
	if err == nil && regionalData != nil && len(regionalData.([]interface{})) > 0 {
		endpointExpression := fmt.Sprintf("[0][?region=='%s'].endpoint", strings.ToLower(param.RegionId))
		var endpointData interface{}
		endpointData, err = jmespath.Search(endpointExpression, regionalData)
		if err == nil && endpointData != nil && len(endpointData.([]interface{})) > 0 {
			endpoint = endpointData.([]interface{})[0].(string)
			support = len(endpoint) > 0
			return
		}
	}
	support = false
	return
}
