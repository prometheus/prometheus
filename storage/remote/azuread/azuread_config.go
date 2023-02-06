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
	"fmt"

	"github.com/google/uuid"
)

// AzureAdConfig is the configuration for getting the accessToken
// for remote write requests to Azure Monitoring Workspace
type AzureAdConfig struct {
	// AzureClientId is the clientId of the managed identity that is being used to authenticate.
	AzureClientId string `yaml:"azure_client_id,omitempty"`

	// Cloud is the Azure cloud in which the service is running. Example: AzurePublic/AzureGovernment/AzureChina
	Cloud string `yaml:"cloud,omitempty"`
}

// Used to validate config values provided
func (c *AzureAdConfig) Validate() error {
	if c.Cloud == "" {
		return fmt.Errorf("must provide Cloud in the Azure AD config")
	}

	if c.AzureClientId == "" {
		return fmt.Errorf("must provide a Azure Managed Identity clientId in the Azure AD config")
	}

	_, err := uuid.Parse(c.AzureClientId)

	if err != nil {
		return fmt.Errorf("Azure Managed Identity clientId provided is invalid")
	}
	return nil
}

// Used to unmarshal Azure Ad config yaml
func (c *AzureAdConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain AzureAdConfig
	*c = AzureAdConfig{}
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}
