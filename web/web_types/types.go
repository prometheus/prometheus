// Copyright 2019 The Prometheus Authors
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

package web_types

import (
	"time"
)

// RuntimeInfo contains runtime info about Prometheus.
type RuntimeInfo struct {
	Birth               time.Time          `json:"startTime"`
	CWD                 string             `json:"cwd"`
	Version             *PrometheusVersion `json:"version"`
	Alertmanagers       []Alertmanager     `json:"alertmanagers"`
	GoroutineCount      int                `json:"goroutines"`
	GOMAXPROCS          int                `json:"goMaxProcs"`
	GOGC                string             `json:"goGC"`
	GODEBUG             string             `json:"goDebug"`
	CorruptionCount     int64              `json:"corruptionCount"`
	ChunkCount          int64              `json:"chunks"`
	TimeSeriesCount     int64              `json:"timeSeries"`
	LastConfigTime      time.Time          `json:"lastConfigTime"`
	ReloadConfigSuccess bool               `json:"reloadConfigSuccess"`
	StorageRetention    string             `json:"storageRetention"`
}

// PrometheusVersion contains build information about Prometheus.
type PrometheusVersion struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	BuildUser string `json:"buildUser"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

type Alertmanager struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Path   string `json:"path"`
}
