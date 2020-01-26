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

package spanner

// DatabasePath returns the path for the database resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, database)
// instead.
func DatabasePath(project, instance, database string) string {
	return "" +
		"projects/" +
		project +
		"/instances/" +
		instance +
		"/databases/" +
		database +
		""
}

// SessionPath returns the path for the session resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/instances/%s/databases/%s/sessions/%s", project, instance, database, session)
// instead.
func SessionPath(project, instance, database, session string) string {
	return "" +
		"projects/" +
		project +
		"/instances/" +
		instance +
		"/databases/" +
		database +
		"/sessions/" +
		session +
		""
}
