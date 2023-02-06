// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package azuread

import (
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// Returns new Managed Identity token credential
func NewManagedIdentityTokenCredential(managedIdentityClientId string) (azcore.TokenCredential, error) {
	if len(managedIdentityClientId) > 0 {
		clientId := azidentity.ClientID(managedIdentityClientId)
		opts := &azidentity.ManagedIdentityCredentialOptions{ID: clientId}
		return azidentity.NewManagedIdentityCredential(opts)
	} else {
		return nil, errors.New("The Managed Identity Client ID can not be empty")
	}
}
