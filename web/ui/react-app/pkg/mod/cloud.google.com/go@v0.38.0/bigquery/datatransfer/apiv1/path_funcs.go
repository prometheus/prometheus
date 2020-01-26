// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datatransfer

// ProjectPath returns the path for the project resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s", project)
// instead.
func ProjectPath(project string) string {
	return "" +
		"projects/" +
		project +
		""
}

// LocationPath returns the path for the location resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/locations/%s", project, location)
// instead.
func LocationPath(project, location string) string {
	return "" +
		"projects/" +
		project +
		"/locations/" +
		location +
		""
}

// LocationDataSourcePath returns the path for the location data source resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/locations/%s/dataSources/%s", project, location, dataSource)
// instead.
func LocationDataSourcePath(project, location, dataSource string) string {
	return "" +
		"projects/" +
		project +
		"/locations/" +
		location +
		"/dataSources/" +
		dataSource +
		""
}

// LocationTransferConfigPath returns the path for the location transfer config resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/locations/%s/transferConfigs/%s", project, location, transferConfig)
// instead.
func LocationTransferConfigPath(project, location, transferConfig string) string {
	return "" +
		"projects/" +
		project +
		"/locations/" +
		location +
		"/transferConfigs/" +
		transferConfig +
		""
}

// LocationRunPath returns the path for the location run resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/locations/%s/transferConfigs/%s/runs/%s", project, location, transferConfig, run)
// instead.
func LocationRunPath(project, location, transferConfig, run string) string {
	return "" +
		"projects/" +
		project +
		"/locations/" +
		location +
		"/transferConfigs/" +
		transferConfig +
		"/runs/" +
		run +
		""
}

// DataSourcePath returns the path for the data source resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/dataSources/%s", project, dataSource)
// instead.
func DataSourcePath(project, dataSource string) string {
	return "" +
		"projects/" +
		project +
		"/dataSources/" +
		dataSource +
		""
}

// TransferConfigPath returns the path for the transfer config resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/transferConfigs/%s", project, transferConfig)
// instead.
func TransferConfigPath(project, transferConfig string) string {
	return "" +
		"projects/" +
		project +
		"/transferConfigs/" +
		transferConfig +
		""
}

// RunPath returns the path for the run resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/transferConfigs/%s/runs/%s", project, transferConfig, run)
// instead.
func RunPath(project, transferConfig, run string) string {
	return "" +
		"projects/" +
		project +
		"/transferConfigs/" +
		transferConfig +
		"/runs/" +
		run +
		""
}
