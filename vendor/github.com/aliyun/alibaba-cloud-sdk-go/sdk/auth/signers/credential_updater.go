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

package signers

import (
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
)

const defaultInAdvanceScale = 0.95

type credentialUpdater struct {
	credentialExpiration int
	lastUpdateTimestamp  int64
	inAdvanceScale       float64
	buildRequestMethod   func() (*requests.CommonRequest, error)
	responseCallBack     func(response *responses.CommonResponse) error
	refreshApi           func(request *requests.CommonRequest) (response *responses.CommonResponse, err error)
}

func (updater *credentialUpdater) needUpdateCredential() (result bool) {
	if updater.inAdvanceScale == 0 {
		updater.inAdvanceScale = defaultInAdvanceScale
	}
	return time.Now().Unix()-updater.lastUpdateTimestamp >= int64(float64(updater.credentialExpiration)*updater.inAdvanceScale)
}

func (updater *credentialUpdater) updateCredential() (err error) {
	request, err := updater.buildRequestMethod()
	if err != nil {
		return
	}
	response, err := updater.refreshApi(request)
	if err != nil {
		return
	}
	updater.lastUpdateTimestamp = time.Now().Unix()
	err = updater.responseCallBack(response)
	return
}
