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
