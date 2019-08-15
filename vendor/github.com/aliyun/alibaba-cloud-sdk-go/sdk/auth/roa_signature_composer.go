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
	"bytes"
	"sort"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

var debug utils.Debug

var hookGetDate = func(fn func() string) string {
	return fn()
}

func init() {
	debug = utils.Init("sdk")
}

func signRoaRequest(request requests.AcsRequest, signer Signer, regionId string) (err error) {
	completeROASignParams(request, signer, regionId)
	stringToSign := buildRoaStringToSign(request)
	request.SetStringToSign(stringToSign)
	accessKeyId, err := signer.GetAccessKeyId()
	if err != nil {
		return err
	}

	signature := signer.Sign(stringToSign, "")
	request.GetHeaders()["Authorization"] = "acs " + accessKeyId + ":" + signature

	return
}

func completeROASignParams(request requests.AcsRequest, signer Signer, regionId string) {
	headerParams := request.GetHeaders()

	// complete query params
	queryParams := request.GetQueryParams()
	//if _, ok := queryParams["RegionId"]; !ok {
	//	queryParams["RegionId"] = regionId
	//}
	if extraParam := signer.GetExtraParam(); extraParam != nil {
		for key, value := range extraParam {
			if key == "SecurityToken" {
				headerParams["x-acs-security-token"] = value
				continue
			}
			if key == "BearerToken" {
				headerParams["x-acs-bearer-token"] = value
				continue
			}
			queryParams[key] = value
		}
	}

	// complete header params
	headerParams["Date"] = hookGetDate(utils.GetTimeInFormatRFC2616)
	headerParams["x-acs-signature-method"] = signer.GetName()
	headerParams["x-acs-signature-version"] = signer.GetVersion()
	if request.GetFormParams() != nil && len(request.GetFormParams()) > 0 {
		formString := utils.GetUrlFormedMap(request.GetFormParams())
		request.SetContent([]byte(formString))
		headerParams["Content-Type"] = requests.Form
	}
	contentMD5 := utils.GetMD5Base64(request.GetContent())
	headerParams["Content-MD5"] = contentMD5
	if _, contains := headerParams["Content-Type"]; !contains {
		headerParams["Content-Type"] = requests.Raw
	}
	switch format := request.GetAcceptFormat(); format {
	case "JSON":
		headerParams["Accept"] = requests.Json
	case "XML":
		headerParams["Accept"] = requests.Xml
	default:
		headerParams["Accept"] = requests.Raw
	}
}

func buildRoaStringToSign(request requests.AcsRequest) (stringToSign string) {

	headers := request.GetHeaders()

	stringToSignBuilder := bytes.Buffer{}
	stringToSignBuilder.WriteString(request.GetMethod())
	stringToSignBuilder.WriteString(requests.HeaderSeparator)

	// append header keys for sign
	appendIfContain(headers, &stringToSignBuilder, "Accept", requests.HeaderSeparator)
	appendIfContain(headers, &stringToSignBuilder, "Content-MD5", requests.HeaderSeparator)
	appendIfContain(headers, &stringToSignBuilder, "Content-Type", requests.HeaderSeparator)
	appendIfContain(headers, &stringToSignBuilder, "Date", requests.HeaderSeparator)

	// sort and append headers witch starts with 'x-acs-'
	var acsHeaders []string
	for key := range headers {
		if strings.HasPrefix(key, "x-acs-") {
			acsHeaders = append(acsHeaders, key)
		}
	}
	sort.Strings(acsHeaders)
	for _, key := range acsHeaders {
		stringToSignBuilder.WriteString(key + ":" + headers[key])
		stringToSignBuilder.WriteString(requests.HeaderSeparator)
	}

	// append query params
	stringToSignBuilder.WriteString(request.BuildQueries())
	stringToSign = stringToSignBuilder.String()
	debug("stringToSign: %s", stringToSign)
	return
}

func appendIfContain(sourceMap map[string]string, target *bytes.Buffer, key, separator string) {
	if value, contain := sourceMap[key]; contain && len(value) > 0 {
		target.WriteString(sourceMap[key])
		target.WriteString(separator)
	}
}
