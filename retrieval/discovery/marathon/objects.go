// Copyright 2015 The Prometheus Authors
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

package marathon

// Task describes one instance of a service running on Marathon.
type Task struct {
	ID    string   `json:"id"`
	Host  string   `json:"host"`
	Ports []uint32 `json:"ports"`
}

// DockerContainer describes a container which uses the docker runtime.
type DockerContainer struct {
	Image string `json:"image"`
}

// Container describes the runtime an app in running in.
type Container struct {
	Docker DockerContainer `json:"docker"`
}

// App describes a service running on Marathon.
type App struct {
	ID           string            `json:"id"`
	Tasks        []Task            `json:"tasks"`
	RunningTasks int               `json:"tasksRunning"`
	Labels       map[string]string `json:"labels"`
	Container    Container         `json:"container"`
}

// AppList is a list of Marathon apps.
type AppList struct {
	Apps []App `json:"apps"`
}
