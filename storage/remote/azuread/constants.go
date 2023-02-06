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

const (
	// Clouds
	AzureChina      = "AzureChina"
	AzureGovernment = "AzureGovernment"
	AzurePublic     = "AzurePublic"

	// Audiences
	INGESTION_CHINA_AUDIENCE      = "https://monitor.azure.cn//.default"
	INGESTION_GOVERNMENT_AUDIENCE = "https://monitor.azure.us//.default"
	INGESTION_PUBLIC_AUDIENCE     = "https://monitor.azure.com//.default"
)
