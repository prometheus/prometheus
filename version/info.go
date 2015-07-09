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

package version

import "github.com/prometheus/client_golang/prometheus"

// Build information. Populated at build-time.
var (
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
	GoVersion string
)

// Map provides the iterable version information.
var Map = map[string]string{
	"version":   Version,
	"revision":  Revision,
	"branch":    Branch,
	"buildUser": BuildUser,
	"buildDate": BuildDate,
	"goVersion": GoVersion,
}

func init() {
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_build_info",
			Help: "A metric with a constant '1' value labeled by version, revision, and branch from which Prometheus was built.",
		},
		[]string{"version", "revision", "branch"},
	)
	buildInfo.WithLabelValues(Version, Revision, Branch).Set(1)

	prometheus.MustRegister(buildInfo)
}
