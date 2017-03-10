package consul

import (
	"testing"

	"github.com/prometheus/prometheus/config"
)

var configuredServiceName = "configuredService"
var nonConfiguredServiceName = "nonConfiguredService"

func TestConfiguredService(t *testing.T) {
	conf := &config.ConsulSDConfig{
		Services: []string{configuredServiceName}}

	consulDiscovery, _ := NewDiscovery(conf)

	if !consulDiscovery.shouldWatch(configuredServiceName) {
		t.Errorf("Expected service %s to be watched", configuredServiceName)
	}
	nonConfiguredServiceName := "nonConfiguredService"
	if consulDiscovery.shouldWatch(nonConfiguredServiceName) {
		t.Errorf("Expected service %s to not be watched", nonConfiguredServiceName)
	}
}

func TestNonConfiguredService(t *testing.T) {
	conf := &config.ConsulSDConfig{}

	consulDiscovery, _ := NewDiscovery(conf)

	if !consulDiscovery.shouldWatch(nonConfiguredServiceName) {
		t.Errorf("Expected service %s to be watched", nonConfiguredServiceName)
	}
}
