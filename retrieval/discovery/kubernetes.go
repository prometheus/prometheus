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

package discovery

import (
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval/discovery/kubernetes"
)

// NewKubernetesDiscovery creates a Kubernetes service discovery based on the passed-in configuration.
func NewKubernetesDiscovery(conf *config.KubernetesSDConfig) (*kubernetes.Discovery, error) {
	kd := &kubernetes.Discovery{
		Conf: conf,
	}
	err := kd.Initialize()
	if err != nil {
		return nil, err
	}
	return kd, nil
}
