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
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func loadAzureAdConfig(filename string) (*AzureAdConfig, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg := AzureAdConfig{}
	if err = yaml.UnmarshalStrict(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func testGoodConfig(t *testing.T, filename string) {
	_, err := loadAzureAdConfig(filename)
	if err != nil {
		t.Fatalf("Unexpected error parsing %s: %s", filename, err)
	}
}

func TestGoodAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_good.yaml"
	testGoodConfig(t, filename)
}

func TestBadClientIdMissingAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_bad_clientidmissing.yaml"
	_, err := loadAzureAdConfig(filename)
	if err == nil {
		t.Fatalf("Did not receive expected error unmarshaling bad azuread config")
	}
	if !strings.Contains(err.Error(), "must provide a Azure Managed Identity clientId in the Azure AD config") {
		t.Errorf("Received unexpected error from unmarshal of %s: %s", filename, err.Error())
	}
}

func TestBadCloudMissingAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_bad_cloudmissing.yaml"
	_, err := loadAzureAdConfig(filename)
	if err == nil {
		t.Fatalf("Did not receive expected error unmarshaling bad azuread config")
	}
	if !strings.Contains(err.Error(), "must provide Cloud in the Azure AD config") {
		t.Errorf("Received unexpected error from unmarshal of %s: %s", filename, err.Error())
	}
}

func TestBadInvalidClientIdAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_bad_invalidclientid.yaml"
	_, err := loadAzureAdConfig(filename)
	if err == nil {
		t.Fatalf("Did not receive expected error unmarshaling bad azuread config")
	}
	if !strings.Contains(err.Error(), "Azure Managed Identity clientId provided is invalid") {
		t.Errorf("Received unexpected error from unmarshal of %s: %s", filename, err.Error())
	}
}
