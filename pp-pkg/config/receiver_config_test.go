// Copyright OpCore

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/pp-pkg/config"
	relabelerconfig "github.com/prometheus/prometheus/pp/go/relabeler/config"
)

func TestRemoteWriteReceiverConfig(t *testing.T) {
	// 	raw := `name: some_remote_write_receiver_1
	// relabel_configs:
	// - source_labels: [__name__]
	//   separator: ;
	//   regex: .*
	//   replacement: $1
	//   action: keep
	// `

	cfg := config.RemoteWriteReceiverConfig{
		NumberOfShards: 1,
		Configs: []*relabelerconfig.InputRelabelerConfig{
			{Name: "name"},
		},
	}
	out, err := yaml.Marshal(&cfg)
	require.NoError(t, err)
	t.Log(string(out))

	type CCC struct {
		NNN                        string                           `yaml:"nnn"`
		RemoteWriteReceiverConfigs config.RemoteWriteReceiverConfig `yaml:",inline"`
	}
	cfg1 := &CCC{
		NNN:                        "contains",
		RemoteWriteReceiverConfigs: cfg,
	}
	out, err = yaml.Marshal(cfg1)
	require.NoError(t, err)
	t.Log(string(out))

	cfg2 := &CCC{
		NNN: "empty",
	}
	out, err = yaml.Marshal(cfg2)
	require.NoError(t, err)
	t.Log(string(out))
}

type RemoteWriteReceiverConfigsSuite struct {
	suite.Suite

	cfg *config.RemoteWriteReceiverConfig
}

func TestRemoteWriteReceiverConfigs(t *testing.T) {
	suite.Run(t, new(RemoteWriteReceiverConfigsSuite))
}

func (s *RemoteWriteReceiverConfigsSuite) SetupTest() {
	s.cfg = new(config.RemoteWriteReceiverConfig)
}

func (s *RemoteWriteReceiverConfigsSuite) TestRemoteWriteReceiverConfigUnmarshal() {
	raw := `number_of_shards: 1
remote_write_receivers:
- name: some_remote_write_receiver_1
  relabel_configs:
  - source_labels: [__name__]
    separator: ;
    regex: .*
    replacement: $1
    action: keep
`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().NoError(err)

	out, err := yaml.Marshal(s.cfg)
	s.Require().NoError(err)

	s.Require().Equal(raw, string(out))
}

func (s *RemoteWriteReceiverConfigsSuite) TestRemoteWriteReceiverConfigUnmarshalNoName() {
	raw := `number_of_shards: 1
remote_write_receivers:
- relabel_configs:
  - source_labels: [__name__]
    separator: ;
    regex: .*
    replacement: $1
    action: keep
`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *RemoteWriteReceiverConfigsSuite) TestRemoteWriteReceiverConfigUnmarshalNoRelabeling() {
	raw := `number_of_shards: 2
remote_write_receivers:
- name: some_remote_write_receiver_1
`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().NoError(err)

	out, err := yaml.Marshal(s.cfg)
	s.Require().NoError(err)

	s.Require().Equal(raw, string(out))
}

func (s *RemoteWriteReceiverConfigsSuite) TestRemoteWriteReceiverConfigUnmarshalMultipleName() {
	raw := `number_of_shards: 2
remote_write_receivers:
- name: some_remote_write_receiver_1
- name: some_remote_write_receiver_1
`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *RemoteWriteReceiverConfigsSuite) TestRemoteWriteReceiverConfigCopy() {
	raw := `number_of_shards: 1
remote_write_receivers:
- name: some_remote_write_receiver_1
  relabel_configs:
  - source_labels: [__name__]
    separator: ;
    regex: .*
    replacement: $1
    action: keep
`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().NoError(err)

	newCfg := s.cfg.Copy()
	copyCfg := s.cfg.Copy()
	copyCfg.Configs[0].Name = "boo"
	out, err := yaml.Marshal(newCfg)
	s.Require().NoError(err)
	s.Require().Equal(raw, string(out))

	newCfg = s.cfg.Copy()
	copyCfg = s.cfg.Copy()
	copyCfg.Configs[0].RelabelConfigs[0].Replacement = "boo"
	out, err = yaml.Marshal(newCfg)
	s.Require().NoError(err)
	s.Require().Equal(raw, string(out))

	newCfg = s.cfg.Copy()
	copyCfg = s.cfg.Copy()
	copyCfg.Configs[0].RelabelConfigs[0].SourceLabels[0] = "boo"
	out, err = yaml.Marshal(newCfg)
	s.Require().NoError(err)
	s.Require().Equal(raw, string(out))
}
