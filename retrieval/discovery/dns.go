// Copyright 2016 The Prometheus Authors
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
	"github.com/prometheus/prometheus/retrieval/discovery/dns"

	"github.com/prometheus/prometheus/config"
)

// NewDNS creates a new DNS based discovery.
func NewDNS(conf *config.DNSSDConfig) *dns.Discovery {
	return dns.NewDiscovery(conf)
}
