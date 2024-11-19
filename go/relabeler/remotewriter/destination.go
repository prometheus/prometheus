package remotewriter

import "github.com/prometheus/prometheus/config"

type DestinationConfig struct {
	config.RemoteWriteConfig
}

func (c DestinationConfig) EqualTo(other DestinationConfig) bool {
	// todo: implement me
	return false
}

func (c DestinationConfig) Hash() uint32 {
	// todo: implement me
	return 0
}

type Destination struct {
	config DestinationConfig
	HeadID *string
}

func (d *Destination) Config() DestinationConfig {
	return d.config
}

func NewDestination(config DestinationConfig) *Destination {
	return &Destination{config: config}
}
