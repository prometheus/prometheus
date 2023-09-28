// Copyright 2020 The Prometheus Authors
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

// Package install has the side-effect of registering all builtin
// service discovery config types.
package install

import (
	_ "github.com/prometheus/prometheus/v2/discovery/aws"          // register aws
	_ "github.com/prometheus/prometheus/v2/discovery/azure"        // register azure
	_ "github.com/prometheus/prometheus/v2/discovery/consul"       // register consul
	_ "github.com/prometheus/prometheus/v2/discovery/digitalocean" // register digitalocean
	_ "github.com/prometheus/prometheus/v2/discovery/dns"          // register dns
	_ "github.com/prometheus/prometheus/v2/discovery/eureka"       // register eureka
	_ "github.com/prometheus/prometheus/v2/discovery/file"         // register file
	_ "github.com/prometheus/prometheus/v2/discovery/gce"          // register gce
	_ "github.com/prometheus/prometheus/v2/discovery/hetzner"      // register hetzner
	_ "github.com/prometheus/prometheus/v2/discovery/http"         // register http
	_ "github.com/prometheus/prometheus/v2/discovery/ionos"        // register ionos
	_ "github.com/prometheus/prometheus/v2/discovery/kubernetes"   // register kubernetes
	_ "github.com/prometheus/prometheus/v2/discovery/linode"       // register linode
	_ "github.com/prometheus/prometheus/v2/discovery/marathon"     // register marathon
	_ "github.com/prometheus/prometheus/v2/discovery/moby"         // register moby
	_ "github.com/prometheus/prometheus/v2/discovery/nomad"        // register nomad
	_ "github.com/prometheus/prometheus/v2/discovery/openstack"    // register openstack
	_ "github.com/prometheus/prometheus/v2/discovery/ovhcloud"     // register ovhcloud
	_ "github.com/prometheus/prometheus/v2/discovery/puppetdb"     // register puppetdb
	_ "github.com/prometheus/prometheus/v2/discovery/scaleway"     // register scaleway
	_ "github.com/prometheus/prometheus/v2/discovery/triton"       // register triton
	_ "github.com/prometheus/prometheus/v2/discovery/uyuni"        // register uyuni
	_ "github.com/prometheus/prometheus/v2/discovery/vultr"        // register vultr
	_ "github.com/prometheus/prometheus/v2/discovery/xds"          // register xds
	_ "github.com/prometheus/prometheus/v2/discovery/zookeeper"    // register zookeeper
)
