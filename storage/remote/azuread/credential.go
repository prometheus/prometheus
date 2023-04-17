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
)

// NewTokenCredential return TokenCredential of different kinds like Azure Managed Identity and Azure AD application.
func NewTokenCredential(cfg *AzureAdConfig) (azcore.TokenCredential, error) {
	var cred azcore.TokenCredential
	var err error
	if len(cfg.AzureClientId) > 0 {
		cred, err = NewManagedIdentityTokenCredential(cfg.AzureClientId)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("Azure Client ID is invalid")
	}

	return cred, nil
}
