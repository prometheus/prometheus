package consul

import (
	"github.com/prometheus/prometheus/config"
	"testing"
)

var configuredServiceName = "configuredService"
var nonConfiguredServiceName = "nonConfiguredService"
var configuredTagName = "configuredTag"
var nonConfiguredTagName = "nonConfiguredTag"

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

func TestNoConfiguredServices(t *testing.T) {
	conf := &config.ConsulSDConfig{}

	consulDiscovery, _ := NewDiscovery(conf)

	if !consulDiscovery.shouldWatch(nonConfiguredServiceName) {
		t.Errorf("Expected service %s to be watched", nonConfiguredServiceName)
	}
}

func TestConfiguredTag(t *testing.T) {
	conf := &config.ConsulSDConfig{
		Tags: []string{configuredTagName}}

	consulDiscovery, _ := NewDiscovery(conf)

	if !consulDiscovery.shouldWatchTags([]string{configuredTagName}) {
		t.Errorf("Expected tag %s to be watched", configuredTagName)
	}
	nonConfiguredTagName := "nonConfiguredTag"
	if consulDiscovery.shouldWatchTags([]string{nonConfiguredTagName}) {
		t.Errorf("Expected tag %s to be watched", nonConfiguredTagName)
	}
}

func TestNoConfiguredTags(t *testing.T) {
	conf := &config.ConsulSDConfig{}

	consulDiscovery, _ := NewDiscovery(conf)

	if !consulDiscovery.shouldWatchTags([]string{nonConfiguredTagName}) {
		t.Errorf("Expected tag %s to be watched", nonConfiguredTagName)
	}
}

func TestConfiguredTagAndService(t *testing.T) {
	conf := &config.ConsulSDConfig{
		Tags:     []string{configuredTagName},
		Services: []string{configuredServiceName}}

	consulDiscovery, _ := NewDiscovery(conf)

	if !consulDiscovery.shouldWatchTags([]string{configuredTagName}) {
		t.Errorf("Expected tag %s to be watched", configuredTagName)
	}
	if consulDiscovery.shouldWatchTags([]string{nonConfiguredTagName}) {
		t.Errorf("Expected tag %s to be watched", nonConfiguredTagName)
	}
}
