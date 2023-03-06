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
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// newTokenCredential returns a TokenCredential of different kinds like Azure Managed Identity and Azure AD application.
func newTokenCredential(cfg *AzureADConfig) (azcore.TokenCredential, error) {
	cred, err := newManagedIdentityTokenCredential(cfg.ClientID)
	if err != nil {
		return nil, err
	}

	return cred, nil
}
