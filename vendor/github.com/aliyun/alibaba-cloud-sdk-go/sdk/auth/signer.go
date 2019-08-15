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
	"fmt"
	"reflect"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/signers"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
)

type Signer interface {
	GetName() string
	GetType() string
	GetVersion() string
	GetAccessKeyId() (string, error)
	GetExtraParam() map[string]string
	Sign(stringToSign, secretSuffix string) string
}

func NewSignerWithCredential(credential Credential, commonApi func(request *requests.CommonRequest, signer interface{}) (response *responses.CommonResponse, err error)) (signer Signer, err error) {
	switch instance := credential.(type) {
	case *credentials.AccessKeyCredential:
		{
			signer = signers.NewAccessKeySigner(instance)
		}
	case *credentials.StsTokenCredential:
		{
			signer = signers.NewStsTokenSigner(instance)
		}
	case *credentials.BearerTokenCredential:
		{
			signer = signers.NewBearerTokenSigner(instance)
		}
	case *credentials.RamRoleArnCredential:
		{
			signer, err = signers.NewRamRoleArnSigner(instance, commonApi)
		}
	case *credentials.RsaKeyPairCredential:
		{
			signer, err = signers.NewSignerKeyPair(instance, commonApi)
		}
	case *credentials.EcsRamRoleCredential:
		{
			signer = signers.NewEcsRamRoleSigner(instance, commonApi)
		}
	case *credentials.BaseCredential: // deprecated user interface
		{
			signer = signers.NewAccessKeySigner(instance.ToAccessKeyCredential())
		}
	case *credentials.StsRoleArnCredential: // deprecated user interface
		{
			signer, err = signers.NewRamRoleArnSigner(instance.ToRamRoleArnCredential(), commonApi)
		}
	case *credentials.StsRoleNameOnEcsCredential: // deprecated user interface
		{
			signer = signers.NewEcsRamRoleSigner(instance.ToEcsRamRoleCredential(), commonApi)
		}
	default:
		message := fmt.Sprintf(errors.UnsupportedCredentialErrorMessage, reflect.TypeOf(credential))
		err = errors.NewClientError(errors.UnsupportedCredentialErrorCode, message, nil)
	}
	return
}

func Sign(request requests.AcsRequest, signer Signer, regionId string) (err error) {
	switch request.GetStyle() {
	case requests.ROA:
		{
			err = signRoaRequest(request, signer, regionId)
		}
	case requests.RPC:
		{
			err = signRpcRequest(request, signer, regionId)
		}
	default:
		message := fmt.Sprintf(errors.UnknownRequestTypeErrorMessage, reflect.TypeOf(request))
		err = errors.NewClientError(errors.UnknownRequestTypeErrorCode, message, nil)
	}

	return
}
