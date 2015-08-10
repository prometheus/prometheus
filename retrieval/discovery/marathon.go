package discovery

import (
	"time"

	"github.com/prometheus/log"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval/discovery/marathon"
)

// MarathonDiscovery provides service discovery based on a Marathon instance.
type MarathonDiscovery struct {
	servers         []string
	refreshInterval time.Duration
	done            chan struct{}
	lastRefresh     map[string]*config.TargetGroup
	client          marathon.AppListClient
}

// NewMarathonDiscovery creates a new Marathon based discovery.
func NewMarathonDiscovery(conf *config.MarathonSDConfig) *MarathonDiscovery {
	return &MarathonDiscovery{
		servers:         conf.Servers,
		refreshInterval: time.Duration(conf.RefreshInterval),
		done:            make(chan struct{}),
		client:          marathon.FetchMarathonApps,
	}
}

// Sources implements the TargetProvider interface.
func (md *MarathonDiscovery) Sources() []string {
	var sources []string
	tgroups, err := md.fetchTargetGroups()
	if err == nil {
		for source := range tgroups {
			sources = append(sources, source)
		}
	}
	return sources
}

// Run implements the TargetProvider interface.
func (md *MarathonDiscovery) Run(ch chan<- *config.TargetGroup, done <-chan struct{}) {
	defer close(ch)

	for {
		select {
		case <-done:
			return
		case <-time.After(md.refreshInterval):
			err := md.updateServices(ch)
			if err != nil {
				log.Errorf("Error while updating services: %s", err)
			}
		}
	}
}

func (md *MarathonDiscovery) updateServices(ch chan<- *config.TargetGroup) error {
	targetMap, err := md.fetchTargetGroups()
	if err != nil {
		return err
	}

	// Update services which are still present
	for _, tg := range targetMap {
		ch <- tg
	}

	// Remove services which did disappear
	for source := range md.lastRefresh {
		_, ok := targetMap[source]
		if !ok {
			log.Debugf("Removing group for %s", source)
			ch <- &config.TargetGroup{Source: source}
		}
	}

	md.lastRefresh = targetMap
	return nil
}

func (md *MarathonDiscovery) fetchTargetGroups() (map[string]*config.TargetGroup, error) {
	url := marathon.RandomAppsURL(md.servers)
	apps, err := md.client(url)
	if err != nil {
		return nil, err
	}

	groups := marathon.AppsToTargetGroups(apps)
	return groups, nil
}
