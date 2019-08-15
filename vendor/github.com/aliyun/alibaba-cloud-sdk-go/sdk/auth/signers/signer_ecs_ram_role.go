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
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	jmespath "github.com/jmespath/go-jmespath"
)

var securityCredURL = "http://100.100.100.200/latest/meta-data/ram/security-credentials/"

type EcsRamRoleSigner struct {
	*credentialUpdater
	sessionCredential *SessionCredential
	credential        *credentials.EcsRamRoleCredential
	commonApi         func(request *requests.CommonRequest, signer interface{}) (response *responses.CommonResponse, err error)
}

func NewEcsRamRoleSigner(credential *credentials.EcsRamRoleCredential, commonApi func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error)) (signer *EcsRamRoleSigner) {
	signer = &EcsRamRoleSigner{
		credential: credential,
		commonApi:  commonApi,
	}

	signer.credentialUpdater = &credentialUpdater{
		credentialExpiration: defaultDurationSeconds / 60,
		buildRequestMethod:   signer.buildCommonRequest,
		responseCallBack:     signer.refreshCredential,
		refreshApi:           signer.refreshApi,
	}

	return signer
}

func (*EcsRamRoleSigner) GetName() string {
	return "HMAC-SHA1"
}

func (*EcsRamRoleSigner) GetType() string {
	return ""
}

func (*EcsRamRoleSigner) GetVersion() string {
	return "1.0"
}

func (signer *EcsRamRoleSigner) GetAccessKeyId() (accessKeyId string, err error) {
	if signer.sessionCredential == nil || signer.needUpdateCredential() {
		err = signer.updateCredential()
		if err != nil {
			return
		}
	}
	if signer.sessionCredential == nil || len(signer.sessionCredential.AccessKeyId) <= 0 {
		return "", nil
	}
	return signer.sessionCredential.AccessKeyId, nil
}

func (signer *EcsRamRoleSigner) GetExtraParam() map[string]string {
	if signer.sessionCredential == nil {
		return make(map[string]string)
	}
	if len(signer.sessionCredential.StsToken) <= 0 {
		return make(map[string]string)
	}
	return map[string]string{"SecurityToken": signer.sessionCredential.StsToken}
}

func (signer *EcsRamRoleSigner) Sign(stringToSign, secretSuffix string) string {
	secret := signer.sessionCredential.AccessKeySecret + secretSuffix
	return ShaHmac1(stringToSign, secret)
}

func (signer *EcsRamRoleSigner) buildCommonRequest() (request *requests.CommonRequest, err error) {
	return
}

func (signer *EcsRamRoleSigner) refreshApi(request *requests.CommonRequest) (response *responses.CommonResponse, err error) {
	requestUrl := securityCredURL + signer.credential.RoleName
	httpRequest, err := http.NewRequest(requests.GET, requestUrl, strings.NewReader(""))
	if err != nil {
		err = fmt.Errorf("refresh Ecs sts token err: %s", err.Error())
		return
	}
	httpClient := &http.Client{}
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		err = fmt.Errorf("refresh Ecs sts token err: %s", err.Error())
		return
	}

	response = responses.NewCommonResponse()
	err = responses.Unmarshal(response, httpResponse, "")
	return
}

func (signer *EcsRamRoleSigner) refreshCredential(response *responses.CommonResponse) (err error) {
	if response.GetHttpStatus() != http.StatusOK {
		return fmt.Errorf("refresh Ecs sts token err, httpStatus: %d, message = %s", response.GetHttpStatus(), response.GetHttpContentString())
	}
	var data interface{}
	err = json.Unmarshal(response.GetHttpContentBytes(), &data)
	if err != nil {
		return fmt.Errorf("refresh Ecs sts token err, json.Unmarshal fail: %s", err.Error())
	}
	code, err := jmespath.Search("Code", data)
	if err != nil {
		return fmt.Errorf("refresh Ecs sts token err, fail to get Code: %s", err.Error())
	}
	if code.(string) != "Success" {
		return fmt.Errorf("refresh Ecs sts token err, Code is not Success")
	}
	accessKeyId, err := jmespath.Search("AccessKeyId", data)
	if err != nil {
		return fmt.Errorf("refresh Ecs sts token err, fail to get AccessKeyId: %s", err.Error())
	}
	accessKeySecret, err := jmespath.Search("AccessKeySecret", data)
	if err != nil {
		return fmt.Errorf("refresh Ecs sts token err, fail to get AccessKeySecret: %s", err.Error())
	}
	securityToken, err := jmespath.Search("SecurityToken", data)
	if err != nil {
		return fmt.Errorf("refresh Ecs sts token err, fail to get SecurityToken: %s", err.Error())
	}
	expiration, err := jmespath.Search("Expiration", data)
	if err != nil {
		return fmt.Errorf("refresh Ecs sts token err, fail to get Expiration: %s", err.Error())
	}
	if accessKeyId == nil || accessKeySecret == nil || securityToken == nil || expiration == nil {
		return
	}

	expirationTime, err := time.Parse("2006-01-02T15:04:05Z", expiration.(string))
	signer.credentialExpiration = int(expirationTime.Unix() - time.Now().Unix())
	signer.sessionCredential = &SessionCredential{
		AccessKeyId:     accessKeyId.(string),
		AccessKeySecret: accessKeySecret.(string),
		StsToken:        securityToken.(string),
	}

	return
}

func (signer *EcsRamRoleSigner) GetSessionCredential() *SessionCredential {
	return signer.sessionCredential
}
