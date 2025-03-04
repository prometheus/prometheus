package config

import (
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// InputRelabelerConfig internal config for input relabelers.
type InputRelabelerConfig struct {
	Name           string                     `yaml:"name"`                      // name of input relabeler, required
	RelabelConfigs []*cppbridge.RelabelConfig `yaml:"relabel_configs,omitempty"` // list of relabeler configs
}

// NewInputRelabelerConfig init new InputRelabelerConfig.
func NewInputRelabelerConfig(name string, configs []*cppbridge.RelabelConfig) *InputRelabelerConfig {
	return &InputRelabelerConfig{name, configs}
}

// Copy return copy *InputRelabelerConfig.
func (c *InputRelabelerConfig) Copy() *InputRelabelerConfig {
	newCfg := &InputRelabelerConfig{
		Name:           c.Name,
		RelabelConfigs: make([]*cppbridge.RelabelConfig, 0, len(c.RelabelConfigs)),
	}

	for _, rcfg := range c.RelabelConfigs {
		newCfg.RelabelConfigs = append(newCfg.RelabelConfigs, rcfg.Copy())
	}

	return newCfg
}

// GetName return name of input relabeler.
func (c *InputRelabelerConfig) GetName() string {
	return c.Name
}

// GetConfigs return configs with rulers relabeling.
func (c *InputRelabelerConfig) GetConfigs() []*cppbridge.RelabelConfig {
	return c.RelabelConfigs
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *InputRelabelerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain InputRelabelerConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Validate validates receiver config.
func (c *InputRelabelerConfig) Validate() error {
	if c.Name == "" {
		return errors.New("name is empty")
	}

	for _, rlcfg := range c.RelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null target relabeling rule in receiver config")
		}
	}

	return nil
}
