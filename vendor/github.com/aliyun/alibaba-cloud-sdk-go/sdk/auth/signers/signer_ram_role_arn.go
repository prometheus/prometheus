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
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	jmespath "github.com/jmespath/go-jmespath"
)

const (
	defaultDurationSeconds = 3600
)

type RamRoleArnSigner struct {
	*credentialUpdater
	roleSessionName   string
	sessionCredential *SessionCredential
	credential        *credentials.RamRoleArnCredential
	commonApi         func(request *requests.CommonRequest, signer interface{}) (response *responses.CommonResponse, err error)
}

func NewRamRoleArnSigner(credential *credentials.RamRoleArnCredential, commonApi func(request *requests.CommonRequest, signer interface{}) (response *responses.CommonResponse, err error)) (signer *RamRoleArnSigner, err error) {
	signer = &RamRoleArnSigner{
		credential: credential,
		commonApi:  commonApi,
	}

	signer.credentialUpdater = &credentialUpdater{
		credentialExpiration: credential.RoleSessionExpiration,
		buildRequestMethod:   signer.buildCommonRequest,
		responseCallBack:     signer.refreshCredential,
		refreshApi:           signer.refreshApi,
	}

	if len(credential.RoleSessionName) > 0 {
		signer.roleSessionName = credential.RoleSessionName
	} else {
		signer.roleSessionName = "aliyun-go-sdk-" + strconv.FormatInt(time.Now().UnixNano()/1000, 10)
	}
	if credential.RoleSessionExpiration > 0 {
		if credential.RoleSessionExpiration >= 900 && credential.RoleSessionExpiration <= 3600 {
			signer.credentialExpiration = credential.RoleSessionExpiration
		} else {
			err = errors.NewClientError(errors.InvalidParamErrorCode, "Assume Role session duration should be in the range of 15min - 1Hr", nil)
		}
	} else {
		signer.credentialExpiration = defaultDurationSeconds
	}
	return
}

func (*RamRoleArnSigner) GetName() string {
	return "HMAC-SHA1"
}

func (*RamRoleArnSigner) GetType() string {
	return ""
}

func (*RamRoleArnSigner) GetVersion() string {
	return "1.0"
}

func (signer *RamRoleArnSigner) GetAccessKeyId() (accessKeyId string, err error) {
	if signer.sessionCredential == nil || signer.needUpdateCredential() {
		err = signer.updateCredential()
		if err != nil {
			return
		}
	}

	if signer.sessionCredential == nil || len(signer.sessionCredential.AccessKeyId) <= 0 {
		return "", err
	}

	return signer.sessionCredential.AccessKeyId, nil
}

func (signer *RamRoleArnSigner) GetExtraParam() map[string]string {
	if signer.sessionCredential == nil || signer.needUpdateCredential() {
		signer.updateCredential()
	}
	if signer.sessionCredential == nil || len(signer.sessionCredential.StsToken) <= 0 {
		return make(map[string]string)
	}
	return map[string]string{"SecurityToken": signer.sessionCredential.StsToken}
}

func (signer *RamRoleArnSigner) Sign(stringToSign, secretSuffix string) string {
	secret := signer.sessionCredential.AccessKeySecret + secretSuffix
	return ShaHmac1(stringToSign, secret)
}

func (signer *RamRoleArnSigner) buildCommonRequest() (request *requests.CommonRequest, err error) {
	request = requests.NewCommonRequest()
	request.Product = "Sts"
	request.Version = "2015-04-01"
	request.ApiName = "AssumeRole"
	request.Scheme = requests.HTTPS
	request.QueryParams["RoleArn"] = signer.credential.RoleArn
	if signer.credential.Policy != "" {
		request.QueryParams["Policy"] = signer.credential.Policy
	}
	request.QueryParams["RoleSessionName"] = signer.credential.RoleSessionName
	request.QueryParams["DurationSeconds"] = strconv.Itoa(signer.credentialExpiration)
	return
}

func (signer *RamRoleArnSigner) refreshApi(request *requests.CommonRequest) (response *responses.CommonResponse, err error) {
	credential := &credentials.AccessKeyCredential{
		AccessKeyId:     signer.credential.AccessKeyId,
		AccessKeySecret: signer.credential.AccessKeySecret,
	}
	signerV1 := NewAccessKeySigner(credential)
	return signer.commonApi(request, signerV1)
}

func (signer *RamRoleArnSigner) refreshCredential(response *responses.CommonResponse) (err error) {
	if response.GetHttpStatus() != http.StatusOK {
		message := "refresh session token failed"
		err = errors.NewServerError(response.GetHttpStatus(), response.GetHttpContentString(), message)
		return
	}
	var data interface{}
	err = json.Unmarshal(response.GetHttpContentBytes(), &data)
	if err != nil {
		return fmt.Errorf("refresh RoleArn sts token err, json.Unmarshal fail: %s", err.Error())
	}
	accessKeyId, err := jmespath.Search("Credentials.AccessKeyId", data)
	if err != nil {
		return fmt.Errorf("refresh RoleArn sts token err, fail to get AccessKeyId: %s", err.Error())
	}
	accessKeySecret, err := jmespath.Search("Credentials.AccessKeySecret", data)
	if err != nil {
		return fmt.Errorf("refresh RoleArn sts token err, fail to get AccessKeySecret: %s", err.Error())
	}
	securityToken, err := jmespath.Search("Credentials.SecurityToken", data)
	if err != nil {
		return fmt.Errorf("refresh RoleArn sts token err, fail to get SecurityToken: %s", err.Error())
	}
	if accessKeyId == nil || accessKeySecret == nil || securityToken == nil {
		return
	}
	signer.sessionCredential = &SessionCredential{
		AccessKeyId:     accessKeyId.(string),
		AccessKeySecret: accessKeySecret.(string),
		StsToken:        securityToken.(string),
	}
	return
}

func (signer *RamRoleArnSigner) GetSessionCredential() *SessionCredential {
	return signer.sessionCredential
}
