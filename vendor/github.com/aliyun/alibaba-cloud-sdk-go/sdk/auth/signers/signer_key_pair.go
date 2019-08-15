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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	jmespath "github.com/jmespath/go-jmespath"
)

type SignerKeyPair struct {
	*credentialUpdater
	sessionCredential *SessionCredential
	credential        *credentials.RsaKeyPairCredential
	commonApi         func(request *requests.CommonRequest, signer interface{}) (response *responses.CommonResponse, err error)
}

func NewSignerKeyPair(credential *credentials.RsaKeyPairCredential, commonApi func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error)) (signer *SignerKeyPair, err error) {
	signer = &SignerKeyPair{
		credential: credential,
		commonApi:  commonApi,
	}

	signer.credentialUpdater = &credentialUpdater{
		credentialExpiration: credential.SessionExpiration,
		buildRequestMethod:   signer.buildCommonRequest,
		responseCallBack:     signer.refreshCredential,
		refreshApi:           signer.refreshApi,
	}

	if credential.SessionExpiration > 0 {
		if credential.SessionExpiration >= 900 && credential.SessionExpiration <= 3600 {
			signer.credentialExpiration = credential.SessionExpiration
		} else {
			err = errors.NewClientError(errors.InvalidParamErrorCode, "Key Pair session duration should be in the range of 15min - 1Hr", nil)
		}
	} else {
		signer.credentialExpiration = defaultDurationSeconds
	}
	return
}

func (*SignerKeyPair) GetName() string {
	return "HMAC-SHA1"
}

func (*SignerKeyPair) GetType() string {
	return ""
}

func (*SignerKeyPair) GetVersion() string {
	return "1.0"
}

func (signer *SignerKeyPair) ensureCredential() error {
	if signer.sessionCredential == nil || signer.needUpdateCredential() {
		return signer.updateCredential()
	}
	return nil
}

func (signer *SignerKeyPair) GetAccessKeyId() (accessKeyId string, err error) {
	err = signer.ensureCredential()
	if err != nil {
		return
	}
	if signer.sessionCredential == nil || len(signer.sessionCredential.AccessKeyId) <= 0 {
		accessKeyId = ""
		return
	}

	accessKeyId = signer.sessionCredential.AccessKeyId
	return
}

func (signer *SignerKeyPair) GetExtraParam() map[string]string {
	return make(map[string]string)
}

func (signer *SignerKeyPair) Sign(stringToSign, secretSuffix string) string {
	secret := signer.sessionCredential.AccessKeySecret + secretSuffix
	return ShaHmac1(stringToSign, secret)
}

func (signer *SignerKeyPair) buildCommonRequest() (request *requests.CommonRequest, err error) {
	request = requests.NewCommonRequest()
	request.Product = "Sts"
	request.Version = "2015-04-01"
	request.ApiName = "GenerateSessionAccessKey"
	request.Scheme = requests.HTTPS
	request.SetDomain("sts.ap-northeast-1.aliyuncs.com")
	request.QueryParams["PublicKeyId"] = signer.credential.PublicKeyId
	request.QueryParams["DurationSeconds"] = strconv.Itoa(signer.credentialExpiration)
	return
}

func (signer *SignerKeyPair) refreshApi(request *requests.CommonRequest) (response *responses.CommonResponse, err error) {
	signerV2 := NewSignerV2(signer.credential)
	return signer.commonApi(request, signerV2)
}

func (signer *SignerKeyPair) refreshCredential(response *responses.CommonResponse) (err error) {
	if response.GetHttpStatus() != http.StatusOK {
		message := "refresh session AccessKey failed"
		err = errors.NewServerError(response.GetHttpStatus(), response.GetHttpContentString(), message)
		return
	}
	var data interface{}
	err = json.Unmarshal(response.GetHttpContentBytes(), &data)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err, json.Unmarshal fail: %s", err.Error())
	}
	accessKeyId, err := jmespath.Search("SessionAccessKey.SessionAccessKeyId", data)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err, fail to get SessionAccessKeyId: %s", err.Error())
	}
	accessKeySecret, err := jmespath.Search("SessionAccessKey.SessionAccessKeySecret", data)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err, fail to get SessionAccessKeySecret: %s", err.Error())
	}
	if accessKeyId == nil || accessKeySecret == nil {
		return
	}
	signer.sessionCredential = &SessionCredential{
		AccessKeyId:     accessKeyId.(string),
		AccessKeySecret: accessKeySecret.(string),
	}
	return
}
