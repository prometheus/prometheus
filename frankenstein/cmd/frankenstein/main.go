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

package main

import (
	"flag"
	"net/http"
	"time"

	"github.com/prometheus/common/log"

	"github.com/prometheus/prometheus/frankenstein"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
)

func main() {
	var (
		listen        string
		consulHost    string
		consulPrefix  string
		remoteTimeout time.Duration
	)

	flag.StringVar(&listen, "web.listen-address", ":9094", "HTTP server listen address.")
	flag.StringVar(&consulHost, "consul.hostname", "localhost:8500", "Hostname and port of Consul.")
	flag.StringVar(&consulPrefix, "consul.prefix", "collectors/", "Prefix for keys in Consul.")
	flag.DurationVar(&remoteTimeout, "remote.timeout", 100*time.Millisecond, "Timeout for downstream injestors.")

	clientFactory := func(hostname string) (storage.SampleAppender, error) {
		storage := remote.New(&remote.Options{
			GenericURL:     hostname,
			StorageTimeout: remoteTimeout,
		})
		storage.Run()
		return storage, nil
	}

	distributor, err := frankenstein.NewDistributor(frankenstein.DistributorConfig{
		ConsulHost:    consulHost,
		ConsulPrefix:  consulPrefix,
		ClientFactory: clientFactory,
	})
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/push", frankenstein.AppenderHandler(distributor))
	http.ListenAndServe(listen, nil)
}
