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

const Replace = relabel.Replace
const Keep = relabel.Keep
const Drop = relabel.Drop
const HashMod = relabel.HashMod
const LabelMap = relabel.LabelMap
const LabelDrop = relabel.LabelDrop
const LabelKeep = relabel.LabelKeep

type Action = relabel.Action
type Config = relabel.Config
type Regexp = relabel.Regexp

var Process = relabel.Process
var MustNewRegexp = relabel.MustNewRegexp
var NewRegexp = relabel.NewRegexp
