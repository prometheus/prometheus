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

package auth

import (
	"net/url"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

var hookGetNonce = func(fn func() string) string {
	return fn()
}

func signRpcRequest(request requests.AcsRequest, signer Signer, regionId string) (err error) {
	err = completeRpcSignParams(request, signer, regionId)
	if err != nil {
		return
	}
	// remove while retry
	if _, containsSign := request.GetQueryParams()["Signature"]; containsSign {
		delete(request.GetQueryParams(), "Signature")
	}
	stringToSign := buildRpcStringToSign(request)
	request.SetStringToSign(stringToSign)
	signature := signer.Sign(stringToSign, "&")
	request.GetQueryParams()["Signature"] = signature

	return
}

func completeRpcSignParams(request requests.AcsRequest, signer Signer, regionId string) (err error) {
	queryParams := request.GetQueryParams()
	queryParams["Version"] = request.GetVersion()
	queryParams["Action"] = request.GetActionName()
	queryParams["Format"] = request.GetAcceptFormat()
	queryParams["Timestamp"] = hookGetDate(utils.GetTimeInFormatISO8601)
	queryParams["SignatureMethod"] = signer.GetName()
	queryParams["SignatureType"] = signer.GetType()
	queryParams["SignatureVersion"] = signer.GetVersion()
	queryParams["SignatureNonce"] = hookGetNonce(utils.GetUUID)
	queryParams["AccessKeyId"], err = signer.GetAccessKeyId()

	if err != nil {
		return
	}

	if _, contains := queryParams["RegionId"]; !contains {
		queryParams["RegionId"] = regionId
	}
	if extraParam := signer.GetExtraParam(); extraParam != nil {
		for key, value := range extraParam {
			queryParams[key] = value
		}
	}

	request.GetHeaders()["Content-Type"] = requests.Form
	formString := utils.GetUrlFormedMap(request.GetFormParams())
	request.SetContent([]byte(formString))

	return
}

func buildRpcStringToSign(request requests.AcsRequest) (stringToSign string) {
	signParams := make(map[string]string)
	for key, value := range request.GetQueryParams() {
		signParams[key] = value
	}
	for key, value := range request.GetFormParams() {
		signParams[key] = value
	}

	stringToSign = utils.GetUrlFormedMap(signParams)
	stringToSign = strings.Replace(stringToSign, "+", "%20", -1)
	stringToSign = strings.Replace(stringToSign, "*", "%2A", -1)
	stringToSign = strings.Replace(stringToSign, "%7E", "~", -1)
	stringToSign = url.QueryEscape(stringToSign)
	stringToSign = request.GetMethod() + "&%2F&" + stringToSign
	return
}
