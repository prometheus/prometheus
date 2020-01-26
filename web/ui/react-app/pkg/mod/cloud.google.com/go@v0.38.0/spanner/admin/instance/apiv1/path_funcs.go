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

package instance

// InstanceAdminProjectPath returns the path for the project resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s", project)
// instead.
func InstanceAdminProjectPath(project string) string {
	return "" +
		"projects/" +
		project +
		""
}

// InstanceAdminInstanceConfigPath returns the path for the instance config resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/instanceConfigs/%s", project, instanceConfig)
// instead.
func InstanceAdminInstanceConfigPath(project, instanceConfig string) string {
	return "" +
		"projects/" +
		project +
		"/instanceConfigs/" +
		instanceConfig +
		""
}

// InstanceAdminInstancePath returns the path for the instance resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/instances/%s", project, instance)
// instead.
func InstanceAdminInstancePath(project, instance string) string {
	return "" +
		"projects/" +
		project +
		"/instances/" +
		instance +
		""
}
