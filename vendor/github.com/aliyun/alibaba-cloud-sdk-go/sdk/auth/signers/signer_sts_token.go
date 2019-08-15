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
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

type StsTokenSigner struct {
	credential *credentials.StsTokenCredential
}

func NewStsTokenSigner(credential *credentials.StsTokenCredential) *StsTokenSigner {
	return &StsTokenSigner{
		credential: credential,
	}
}

func (*StsTokenSigner) GetName() string {
	return "HMAC-SHA1"
}

func (*StsTokenSigner) GetType() string {
	return ""
}

func (*StsTokenSigner) GetVersion() string {
	return "1.0"
}

func (signer *StsTokenSigner) GetAccessKeyId() (accessKeyId string, err error) {
	return signer.credential.AccessKeyId, nil
}

func (signer *StsTokenSigner) GetExtraParam() map[string]string {
	return map[string]string{"SecurityToken": signer.credential.AccessKeyStsToken}
}

func (signer *StsTokenSigner) Sign(stringToSign, secretSuffix string) string {
	secret := signer.credential.AccessKeySecret + secretSuffix
	return ShaHmac1(stringToSign, secret)
}
