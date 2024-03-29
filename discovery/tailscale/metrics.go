package tailscale

import (
	"github.com/prometheus/prometheus/discovery"
)

var _ discovery.DiscovererMetrics = (*tailscaleMetrics)(nil)

type tailscaleMetrics struct {
	refreshMetrics discovery.RefreshMetricsInstantiator
}

// Register implements discovery.DiscovererMetrics.
func (m *tailscaleMetrics) Register() error {
	return nil
}

// Unregister implements discovery.DiscovererMetrics.
func (m *tailscaleMetrics) Unregister() {}
