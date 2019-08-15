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
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
)

func ShaHmac1(source, secret string) string {
	key := []byte(secret)
	hmac := hmac.New(sha1.New, key)
	hmac.Write([]byte(source))
	signedBytes := hmac.Sum(nil)
	signedString := base64.StdEncoding.EncodeToString(signedBytes)
	return signedString
}

func Sha256WithRsa(source, secret string) string {
	// block, _ := pem.Decode([]byte(secret))
	decodeString, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		panic(err)
	}
	private, err := x509.ParsePKCS8PrivateKey(decodeString)
	if err != nil {
		panic(err)
	}

	h := crypto.Hash.New(crypto.SHA256)
	h.Write([]byte(source))
	hashed := h.Sum(nil)
	signature, err := rsa.SignPKCS1v15(rand.Reader, private.(*rsa.PrivateKey),
		crypto.SHA256, hashed)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(signature)
}
