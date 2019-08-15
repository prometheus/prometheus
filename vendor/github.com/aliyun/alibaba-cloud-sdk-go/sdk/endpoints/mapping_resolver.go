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
)

const keyFormatter = "%s::%s"

var endpointMapping = make(map[string]string)

// AddEndpointMapping Use product id and region id as key to store the endpoint into inner map
func AddEndpointMapping(regionId, productId, endpoint string) (err error) {
	key := fmt.Sprintf(keyFormatter, strings.ToLower(regionId), strings.ToLower(productId))
	endpointMapping[key] = endpoint
	return nil
}

// MappingResolver the mapping resolver type
type MappingResolver struct {
}

// GetName get the resolver name: "mapping resolver"
func (resolver *MappingResolver) GetName() (name string) {
	name = "mapping resolver"
	return
}

// TryResolve use Product and RegionId as key to find endpoint from inner map
func (resolver *MappingResolver) TryResolve(param *ResolveParam) (endpoint string, support bool, err error) {
	key := fmt.Sprintf(keyFormatter, strings.ToLower(param.RegionId), strings.ToLower(param.Product))
	endpoint, contains := endpointMapping[key]
	return endpoint, contains, nil
}
