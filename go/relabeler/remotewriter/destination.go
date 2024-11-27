package remotewriter

import "github.com/prometheus/prometheus/config"

type DestinationConfig struct {
	config.RemoteWriteConfig
}

func (c DestinationConfig) EqualTo(other DestinationConfig) bool {
	// todo: implement me
	return false
}

func (c DestinationConfig) CRC32() (uint32, error) {
	// todo: implement me
	return 0, nil
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
