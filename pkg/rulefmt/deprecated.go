// Package rulefmt is deprecated and replaced with github.com/prometheus/prometheus/model/rulefmt.
package rulefmt

import "github.com/prometheus/prometheus/model/rulefmt"

type Error = rulefmt.Error
type Rule = rulefmt.Rule
type RuleGroup = rulefmt.RuleGroup
type RuleGroups = rulefmt.RuleGroups
type RuleNode = rulefmt.RuleNode
type WrappedError = rulefmt.WrappedError

var Parse = rulefmt.Parse
var ParseFile = rulefmt.ParseFile
