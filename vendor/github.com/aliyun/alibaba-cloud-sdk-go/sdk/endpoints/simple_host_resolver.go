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

// SimpleHostResolver the simple host resolver type
type SimpleHostResolver struct {
}

// GetName get the resolver name: "simple host resolver"
func (resolver *SimpleHostResolver) GetName() (name string) {
	name = "simple host resolver"
	return
}

// TryResolve if the Domain exist in param, use it as endpoint
func (resolver *SimpleHostResolver) TryResolve(param *ResolveParam) (endpoint string, support bool, err error) {
	if support = len(param.Domain) > 0; support {
		endpoint = param.Domain
	}
	return
}
