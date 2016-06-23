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

package main

import (
	"flag"

	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/frankenstein"
)

func main() {
	var (
		listen       string
		consulHost   string
		consulPrefix string
	)

	flag.StringVar(&listen, "web.listen-address", ":9094", "HTTP server listen address.")
	flag.StringVar(&consulHost, "consul.hostname", "consul:8500", "Hostname and port of Consul.")
	flag.StringVar(&consulPrefix, "consul.prefix", "collectors/", "Prefix for keys in Consul.")

	_, err := frankenstein.NewDistributor(frankenstein.DistributorConfig{
		ConsulHost:   consulHost,
		ConsulPrefix: consulPrefix,
	})
	if err != nil {
		log.Fatal(err)
	}
}
