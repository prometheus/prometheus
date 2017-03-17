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

package zookeeper

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/prometheus/common/model"
	"github.com/samuel/go-zookeeper/zk"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"github.com/prometheus/prometheus/util/treecache"
)

// Discovery implements the TargetProvider interface for discovering
// targets from Zookeeper.
type Discovery struct {
	conn *zk.Conn

	sources map[string]*config.TargetGroup

	updates    chan treecache.ZookeeperTreeCacheEvent
	treeCaches []*treecache.ZookeeperTreeCache

	parse func(data []byte, path string) (model.LabelSet, error)
}

// NewNerveDiscovery returns a new Discovery for the given Nerve config.
func NewNerveDiscovery(conf *config.NerveSDConfig) *Discovery {
	return NewDiscovery(conf.Servers, time.Duration(conf.Timeout), conf.Paths, parseNerveMember)
}

// NewServersetDiscovery returns a new Discovery for the given serverset config.
func NewServersetDiscovery(conf *config.ServersetSDConfig) *Discovery {
	return NewDiscovery(conf.Servers, time.Duration(conf.Timeout), conf.Paths, parseServersetMember)
}

// NewDiscovery returns a new discovery along Zookeeper parses with
// the given parse function.
func NewDiscovery(
	srvs []string,
	timeout time.Duration,
	paths []string,
	pf func(data []byte, path string) (model.LabelSet, error),
) *Discovery {
	conn, _, err := zk.Connect(srvs, timeout)
	conn.SetLogger(treecache.ZookeeperLogger{})
	if err != nil {
		return nil
	}
	updates := make(chan treecache.ZookeeperTreeCacheEvent)
	sd := &Discovery{
		conn:    conn,
		updates: updates,
		sources: map[string]*config.TargetGroup{},
		parse:   pf,
	}
	for _, path := range paths {
		sd.treeCaches = append(sd.treeCaches, treecache.NewZookeeperTreeCache(conn, path, updates))
	}
	return sd
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer func() {
		for _, tc := range d.treeCaches {
			tc.Stop()
		}
		// Drain event channel in case the treecache leaks goroutines otherwise.
		for range d.updates {
		}
		d.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
		case event := <-d.updates:
			tg := &config.TargetGroup{
				Source: event.Path,
			}
			if event.Data != nil {
				labelSet, err := d.parse(*event.Data, event.Path)
				if err == nil {
					tg.Targets = []model.LabelSet{labelSet}
					d.sources[event.Path] = tg
				} else {
					delete(d.sources, event.Path)
				}
			} else {
				delete(d.sources, event.Path)
			}
			select {
			case <-ctx.Done():
				return
			case ch <- []*config.TargetGroup{tg}:
			}
		}
	}
}

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

func parseServersetMember(data []byte, path string) (model.LabelSet, error) {
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

	return labels, nil
}

const (
	nerveLabelPrefix         = model.MetaLabelPrefix + "nerve_"
	nervePathLabel           = nerveLabelPrefix + "path"
	nerveEndpointLabelPrefix = nerveLabelPrefix + "endpoint"
)

type nerveMember struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Name string `json:"name"`
}

func parseNerveMember(data []byte, path string) (model.LabelSet, error) {
	member := nerveMember{}
	err := json.Unmarshal(data, &member)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling nerve member %q: %s", path, err)
	}

	labels := model.LabelSet{}
	labels[nervePathLabel] = model.LabelValue(path)
	labels[model.AddressLabel] = model.LabelValue(
		net.JoinHostPort(member.Host, fmt.Sprintf("%d", member.Port)))

	labels[nerveEndpointLabelPrefix+"_host"] = model.LabelValue(member.Host)
	labels[nerveEndpointLabelPrefix+"_port"] = model.LabelValue(fmt.Sprintf("%d", member.Port))
	labels[nerveEndpointLabelPrefix+"_name"] = model.LabelValue(member.Name)

	return labels, nil
}
