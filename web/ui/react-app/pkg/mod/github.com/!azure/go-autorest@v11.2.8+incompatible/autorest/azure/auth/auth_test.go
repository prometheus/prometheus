// Copyright 2017 Microsoft Corporation
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package auth

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	expectedFile = file{
		ClientID:                "client-id-123",
		ClientSecret:            "client-secret-456",
		SubscriptionID:          "sub-id-789",
		TenantID:                "tenant-id-123",
		ActiveDirectoryEndpoint: "https://login.microsoftonline.com",
		ResourceManagerEndpoint: "https://management.azure.com/",
		GraphResourceID:         "https://graph.windows.net/",
		SQLManagementEndpoint:   "https://management.core.windows.net:8443/",
		GalleryEndpoint:         "https://gallery.azure.com/",
		ManagementEndpoint:      "https://management.core.windows.net/",
	}
)

func TestNewAuthorizerFromFile(t *testing.T) {
	os.Setenv("AZURE_AUTH_LOCATION", "./testdata/credsutf16le.json")
	authorizer, err := NewAuthorizerFromFile("https://management.azure.com")
	if err != nil || authorizer == nil {
		t.Logf("NewAuthorizerFromFile failed, got error %v", err)
		t.Fail()
	}
}

func TestNewAuthorizerFromFileWithResource(t *testing.T) {
	os.Setenv("AZURE_AUTH_LOCATION", "./testdata/credsutf16le.json")
	authorizer, err := NewAuthorizerFromFileWithResource("https://my.vault.azure.net")
	if err != nil || authorizer == nil {
		t.Logf("NewAuthorizerFromFileWithResource failed, got error %v", err)
		t.Fail()
	}
}

func TestNewAuthorizerFromEnvironment(t *testing.T) {
	os.Setenv("AZURE_TENANT_ID", expectedFile.TenantID)
	os.Setenv("AZURE_CLIENT_ID", expectedFile.ClientID)
	os.Setenv("AZURE_CLIENT_SECRET", expectedFile.ClientSecret)
	authorizer, err := NewAuthorizerFromEnvironment()

	if err != nil || authorizer == nil {
		t.Logf("NewAuthorizerFromEnvironment failed, got error %v", err)
		t.Fail()
	}
}

func TestNewAuthorizerFromEnvironmentWithResource(t *testing.T) {
	os.Setenv("AZURE_TENANT_ID", expectedFile.TenantID)
	os.Setenv("AZURE_CLIENT_ID", expectedFile.ClientID)
	os.Setenv("AZURE_CLIENT_SECRET", expectedFile.ClientSecret)
	authorizer, err := NewAuthorizerFromEnvironmentWithResource("https://my.vault.azure.net")

	if err != nil || authorizer == nil {
		t.Logf("NewAuthorizerFromEnvironmentWithResource failed, got error %v", err)
		t.Fail()
	}
}

func TestDecodeAndUnmarshal(t *testing.T) {
	tests := []string{
		"credsutf8.json",
		"credsutf16le.json",
		"credsutf16be.json",
	}
	for _, test := range tests {
		b, err := ioutil.ReadFile(filepath.Join("./testdata/", test))
		if err != nil {
			t.Logf("error reading file '%s': %s", test, err)
			t.Fail()
		}
		decoded, err := decode(b)
		if err != nil {
			t.Logf("error decoding file '%s': %s", test, err)
			t.Fail()
		}
		var got file
		err = json.Unmarshal(decoded, &got)
		if err != nil {
			t.Logf("error unmarshaling file '%s': %s", test, err)
			t.Fail()
		}
		if !reflect.DeepEqual(expectedFile, got) {
			t.Logf("unmarshaled map expected %v, got %v", expectedFile, got)
			t.Fail()
		}
	}
}

func areMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}
