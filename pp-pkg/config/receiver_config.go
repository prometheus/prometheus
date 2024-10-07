// Copyright OpCore

package config

import (
	"fmt"

	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

// RemoteWriteReceiverConfig config for remote write receiver.
type RemoteWriteReceiverConfig struct {
	NumberOfShards uint16                         `yaml:"number_of_shards,omitempty"`
	Configs        []*config.InputRelabelerConfig `yaml:"remote_write_receivers,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteWriteReceiverConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain RemoteWriteReceiverConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Copy return copy RemoteWriteReceiverConfig.
func (c *RemoteWriteReceiverConfig) Copy() *RemoteWriteReceiverConfig {
	newCfg := &RemoteWriteReceiverConfig{
		NumberOfShards: c.NumberOfShards,
		Configs:        make([]*config.InputRelabelerConfig, 0, len(c.Configs)),
	}
	for _, icfg := range c.Configs {
		newCfg.Configs = append(newCfg.Configs, icfg.Copy())
	}

	return newCfg
}

// IsEmpty check config is empty.
func (c *RemoteWriteReceiverConfig) IsEmpty() bool {
	return c == nil || len(c.Configs) == 0
}

func (c *RemoteWriteReceiverConfig) Validate() error {
	rNames := map[string]struct{}{}
	for _, rcfg := range c.Configs {
		if _, ok := rNames[rcfg.Name]; ok {
			return fmt.Errorf("found multiple input relabeler configs with job name %q", rcfg.Name)
		}
		rNames[rcfg.Name] = struct{}{}
	}

	if len(c.Configs) != 0 && c.NumberOfShards == 0 {
		c.NumberOfShards = 2
	}

	return nil
}
