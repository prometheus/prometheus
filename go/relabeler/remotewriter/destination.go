package remotewriter

import (
	"bytes"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"gopkg.in/yaml.v2"
	"hash/crc32"
	"time"
)

type DestinationConfig struct {
	config.RemoteWriteConfig
	ExternalLabels labels.Labels `yaml:"external_labels"`
	ReadTimeout    time.Duration
}

func (c DestinationConfig) EqualTo(other DestinationConfig) bool {
	return c.ExternalLabels.Hash() == other.ExternalLabels.Hash() && remoteWriteConfigsAreEqual(c.RemoteWriteConfig, other.RemoteWriteConfig)
}

func (c DestinationConfig) CRC32() (uint32, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return 0, err
	}

	return crc32.ChecksumIEEE(data), nil
}

type Destination struct {
	config DestinationConfig
	HeadID *string
}

func (d *Destination) Config() DestinationConfig {
	return d.config
}

func (d *Destination) ResetConfig(config DestinationConfig) {
	d.config = config
}

func NewDestination(config DestinationConfig) *Destination {
	return &Destination{config: config}
}

func remoteWriteConfigsAreEqual(lrwc, rwrc config.RemoteWriteConfig) bool {
	ldata, _ := yaml.Marshal(lrwc)
	rdata, _ := yaml.Marshal(rwrc)
	return bytes.Equal(ldata, rdata)
}
