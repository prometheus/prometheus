package head

import (
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

type ConfigSource interface {
	Config() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16)
}

type ConfigSourceFunc func() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16)

func (fn ConfigSourceFunc) Config() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) {
	return fn()
}

type BuildFunc func(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (relabeler.Head, error)

type Builder struct {
	configSource ConfigSource
	buildFunc    BuildFunc
}

func NewBuilder(configSource ConfigSource, buildFunc BuildFunc) *Builder {
	return &Builder{
		configSource: configSource,
		buildFunc:    buildFunc,
	}
}

func (b *Builder) Build() (relabeler.Head, error) {
	return b.buildFunc(b.configSource.Config())
}

func (b *Builder) BuildWithConfig(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (relabeler.Head, error) {
	return b.buildFunc(inputRelabelerConfigs, numberOfShards)
}
