// +build go1.9

// Copyright 2018 Microsoft Corporation and contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"os"
	"path"
	"strings"
)

// AzureSDKforGoLocation returns the default location for the Azure-SDK-for-Go to reside.
func AzureSDKforGoLocation() string {
	raw := path.Join(
		os.Getenv("GOPATH"),
		"src",
		"github.com",
		"Azure",
		"azure-sdk-for-go",
	)

	return strings.Replace(raw, "\\", "/", -1)
}

// DefaultOutputLocation establishes the location where profiles should be
// output to unless otherwise specified.
func DefaultOutputLocation() string {
	return path.Join(AzureSDKforGoLocation(), "profiles")
}

// DefaultInputRoot establishes the location where we expect to find the packages
// to create aliases for.
func DefaultInputRoot() string {
	return path.Join(AzureSDKforGoLocation(), "services")
}
