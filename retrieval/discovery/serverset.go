// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/samuel/go-zookeeper/zk"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"github.com/prometheus/prometheus/util/treecache"
)

const (
	serversetLabelPrefix         = model.MetaLabelPrefix + "serverset_"
	serversetStatusLabel         = serversetLabelPrefix + "status"
	serversetPathLabel           = serversetLabelPrefix + "path"
	serversetEndpointLabelPrefix = serversetLabelPrefix + "endpoint"
	serversetShardLabel          = serversetLabelPrefix + "shard"
)

type serversetMember struct {
	ServiceEndpoint     serversetEndpoint
	AdditionalEndpoints map[string]serversetEndpoint
	Status              string `json:"status"`
	Shard               int    `json:"shard"`
}

type serversetEndpoint struct {
	Host string
	Port int
}

// ServersetDiscovery retrieves target information from a Serverset server
// and updates them via watches.
type ServersetDiscovery struct {
	conf       *config.ServersetSDConfig
	conn       *zk.Conn
	mu         sync.RWMutex
	sources    map[string]*config.TargetGroup
	sdUpdates  *chan<- []*config.TargetGroup
	updates    chan treecache.ZookeeperTreeCacheEvent
	treeCaches []*treecache.ZookeeperTreeCache
}

// NewServersetDiscovery returns a new ServersetDiscovery for the given config.
func NewServersetDiscovery(conf *config.ServersetSDConfig) *ServersetDiscovery {
	conn, _, err := zk.Connect(conf.Servers, time.Duration(conf.Timeout))
	conn.SetLogger(treecache.ZookeeperLogger{})
	if err != nil {
		return nil
	}
	updates := make(chan treecache.ZookeeperTreeCacheEvent)
	sd := &ServersetDiscovery{
		conf:    conf,
		conn:    conn,
		updates: updates,
		sources: map[string]*config.TargetGroup{},
	}
	go sd.processUpdates()
	for _, path := range conf.Paths {
		sd.treeCaches = append(sd.treeCaches, treecache.NewZookeeperTreeCache(conn, path, updates))
	}
	return sd
}

func (sd *ServersetDiscovery) processUpdates() {
	defer sd.conn.Close()
	for event := range sd.updates {
		tg := &config.TargetGroup{
			Source: event.Path,
		}
		sd.mu.Lock()
		if event.Data != nil {
			labelSet, err := parseServersetMember(*event.Data, event.Path)
			if err == nil {
				tg.Targets = []model.LabelSet{*labelSet}
				sd.sources[event.Path] = tg
			} else {
				delete(sd.sources, event.Path)
			}
		} else {
			delete(sd.sources, event.Path)
		}
		sd.mu.Unlock()
		if sd.sdUpdates != nil {
			*sd.sdUpdates <- []*config.TargetGroup{tg}
		}
	}

	if sd.sdUpdates != nil {
		close(*sd.sdUpdates)
	}
}

// Run implements the TargetProvider interface.
func (sd *ServersetDiscovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Send on everything we have seen so far.
	sd.mu.Lock()

	all := make([]*config.TargetGroup, 0, len(sd.sources))

	for _, tg := range sd.sources {
		all = append(all, tg)
	}
	ch <- all

	// Tell processUpdates to send future updates.
	sd.sdUpdates = &ch
	sd.mu.Unlock()

	<-ctx.Done()
	for _, tc := range sd.treeCaches {
		tc.Stop()
	}
}

func parseServersetMember(data []byte, path string) (*model.LabelSet, error) {
	member := serversetMember{}

	if err := json.Unmarshal(data, &member); err != nil {
		return nil, fmt.Errorf("error unmarshaling serverset member %q: %s", path, err)
	}

	labels := model.LabelSet{}
	labels[serversetPathLabel] = model.LabelValue(path)
	labels[model.AddressLabel] = model.LabelValue(
		net.JoinHostPort(member.ServiceEndpoint.Host, fmt.Sprintf("%d", member.ServiceEndpoint.Port)))

	labels[serversetEndpointLabelPrefix+"_host"] = model.LabelValue(member.ServiceEndpoint.Host)
	labels[serversetEndpointLabelPrefix+"_port"] = model.LabelValue(fmt.Sprintf("%d", member.ServiceEndpoint.Port))

	for name, endpoint := range member.AdditionalEndpoints {
		cleanName := model.LabelName(strutil.SanitizeLabelName(name))
		labels[serversetEndpointLabelPrefix+"_host_"+cleanName] = model.LabelValue(
			endpoint.Host)
		labels[serversetEndpointLabelPrefix+"_port_"+cleanName] = model.LabelValue(
			fmt.Sprintf("%d", endpoint.Port))

	}

	labels[serversetStatusLabel] = model.LabelValue(member.Status)
	labels[serversetShardLabel] = model.LabelValue(strconv.Itoa(member.Shard))

	return &labels, nil
}
