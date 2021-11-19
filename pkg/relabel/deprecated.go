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
