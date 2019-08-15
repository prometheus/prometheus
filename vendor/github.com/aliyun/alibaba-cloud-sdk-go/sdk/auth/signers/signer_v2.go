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

type SignerV2 struct {
	credential *credentials.RsaKeyPairCredential
}

func (signer *SignerV2) GetExtraParam() map[string]string {
	return nil
}

func NewSignerV2(credential *credentials.RsaKeyPairCredential) *SignerV2 {
	return &SignerV2{
		credential: credential,
	}
}

func (*SignerV2) GetName() string {
	return "SHA256withRSA"
}

func (*SignerV2) GetType() string {
	return "PRIVATEKEY"
}

func (*SignerV2) GetVersion() string {
	return "1.0"
}

func (signer *SignerV2) GetAccessKeyId() (accessKeyId string, err error) {
	return signer.credential.PublicKeyId, err
}

func (signer *SignerV2) Sign(stringToSign, secretSuffix string) string {
	secret := signer.credential.PrivateKey
	return Sha256WithRsa(stringToSign, secret)
}
