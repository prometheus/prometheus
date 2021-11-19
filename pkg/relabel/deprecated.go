// Copyright 2021 The Prometheus Authors
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

// Package relabel is deprecated and replaced with github.com/prometheus/prometheus/model/relabel.
package relabel

import "github.com/prometheus/prometheus/model/relabel"

var DefaultRelabelConfig = relabel.DefaultRelabelConfig

const (
	Replace   = relabel.Replace
	Keep      = relabel.Keep
	Drop      = relabel.Drop
	HashMod   = relabel.HashMod
	LabelMap  = relabel.LabelMap
	LabelDrop = relabel.LabelDrop
	LabelKeep = relabel.LabelKeep
)

type (
	Action = relabel.Action
	Config = relabel.Config
	Regexp = relabel.Regexp
)

var (
	Process       = relabel.Process
	MustNewRegexp = relabel.MustNewRegexp
	NewRegexp     = relabel.NewRegexp
)
