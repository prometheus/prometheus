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

// AzureADConfig is the configuration for getting the accessToken
// for remote write requests to Azure Monitoring Workspace.
type AzureADConfig struct {
	// ClientID is the clientId of the managed identity that is being used to authenticate.
	ClientID string `yaml:"client_id,omitempty"`

	// Cloud is the Azure cloud in which the service is running. Example: AzurePublic/AzureGovernment/AzureChina.
	Cloud string `yaml:"cloud,omitempty"`
}

// Validate validates config values provided.
func (c *AzureADConfig) Validate() error {
	if c.Cloud == "" {
		return fmt.Errorf("must provide a cloud in the Azure AD config")
	}

	if c.ClientID == "" {
		return fmt.Errorf("must provide an Azure Managed Identity client_id in the Azure AD config")
	}

	_, err := uuid.Parse(c.ClientID)

	if err != nil {
		return fmt.Errorf("the provided Azure Managed Identity client_id provided is invalid")
	}
	return nil
}

// UnmarshalYAML unmarshal the Azure AD config yaml.
func (c *AzureADConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain AzureADConfig
	*c = AzureADConfig{}
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}
