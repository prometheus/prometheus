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

package pubsub

// PublisherProjectPath returns the path for the project resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s", project)
// instead.
func PublisherProjectPath(project string) string {
	return "" +
		"projects/" +
		project +
		""
}

// PublisherTopicPath returns the path for the topic resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/topics/%s", project, topic)
// instead.
func PublisherTopicPath(project, topic string) string {
	return "" +
		"projects/" +
		project +
		"/topics/" +
		topic +
		""
}

// SubscriberProjectPath returns the path for the project resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s", project)
// instead.
func SubscriberProjectPath(project string) string {
	return "" +
		"projects/" +
		project +
		""
}

// SubscriberSnapshotPath returns the path for the snapshot resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/snapshots/%s", project, snapshot)
// instead.
func SubscriberSnapshotPath(project, snapshot string) string {
	return "" +
		"projects/" +
		project +
		"/snapshots/" +
		snapshot +
		""
}

// SubscriberSubscriptionPath returns the path for the subscription resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/subscriptions/%s", project, subscription)
// instead.
func SubscriberSubscriptionPath(project, subscription string) string {
	return "" +
		"projects/" +
		project +
		"/subscriptions/" +
		subscription +
		""
}

// SubscriberTopicPath returns the path for the topic resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/topics/%s", project, topic)
// instead.
func SubscriberTopicPath(project, topic string) string {
	return "" +
		"projects/" +
		project +
		"/topics/" +
		topic +
		""
}
