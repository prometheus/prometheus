// Copyright 2013 Prometheus Team
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
	"text/template"
)

// Build information. Populated by Makefile.
var (
	buildVersion    string
	buildBranch     string
	buildUser       string
	buildDate       string
	goVersion       string
	leveldbVersion  string
	protobufVersion string
	snappyVersion   string
)

var BuildInfo = map[string]string{
	"version":          buildVersion,
	"branch":           buildBranch,
	"user":             buildUser,
	"date":             buildDate,
	"go_version":       goVersion,
	"leveldb_version":  leveldbVersion,
	"protobuf_version": protobufVersion,
	"snappy_version":   snappyVersion,
}

var versionInfoTmpl = template.Must(template.New("version").Parse(
	`prometheus, version {{.version}} ({{.branch}})
  build user:       {{.user}}
  build date:       {{.date}}
  go version:       {{.go_version}}
  leveldb version:  {{.leveldb_version}}
  protobuf version: {{.protobuf_version}}
  snappy version:   {{.snappy_version}}
`))
