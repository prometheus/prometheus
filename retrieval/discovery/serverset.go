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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/log"
	"github.com/samuel/go-zookeeper/zk"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	serversetNodePrefix = "member_"

	serversetLabelPrefix         = clientmodel.MetaLabelPrefix + "serverset_"
	serversetStatusLabel         = serversetLabelPrefix + "status"
	serversetPathLabel           = serversetLabelPrefix + "path"
	serversetEndpointLabelPrefix = serversetLabelPrefix + "endpoint"
)

type serversetMember struct {
	ServiceEndpoint     serversetEndpoint
	AdditionalEndpoints map[string]serversetEndpoint
	Status              string `json:"status"`
}

type serversetEndpoint struct {
	Host string
	Port int
}

type ZookeeperLogger struct {
}

// Implements zk.Logger
func (zl ZookeeperLogger) Printf(s string, i ...interface{}) {
	log.Infof(s, i...)
}

// ServersetDiscovery retrieves target information from a Serverset server
// and updates them via watches.
type ServersetDiscovery struct {
	conf      *config.ServersetSDConfig
	conn      *zk.Conn
	mu        sync.RWMutex
	sources   map[string]*config.TargetGroup
	sdUpdates *chan<- *config.TargetGroup
	updates   chan zookeeperTreeCacheEvent
	treeCache *zookeeperTreeCache
}

// NewServersetDiscovery returns a new ServersetDiscovery for the given config.
func NewServersetDiscovery(conf *config.ServersetSDConfig) *ServersetDiscovery {
	conn, _, err := zk.Connect(conf.Servers, time.Duration(conf.Timeout))
	conn.SetLogger(ZookeeperLogger{})
	if err != nil {
		return nil
	}
	updates := make(chan zookeeperTreeCacheEvent)
	sd := &ServersetDiscovery{
		conf:    conf,
		conn:    conn,
		updates: updates,
		sources: map[string]*config.TargetGroup{},
	}
	go sd.processUpdates()
	sd.treeCache = NewZookeeperTreeCache(conn, conf.Paths[0], updates)
	return sd
}

// Sources implements the TargetProvider interface.
func (sd *ServersetDiscovery) Sources() []string {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	srcs := []string{}
	for t := range sd.sources {
		srcs = append(srcs, t)
	}
	return srcs
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
				tg.Targets = []clientmodel.LabelSet{*labelSet}
				sd.sources[event.Path] = tg
			} else {
				delete(sd.sources, event.Path)
			}
		} else {
			delete(sd.sources, event.Path)
		}
		sd.mu.Unlock()
		if sd.sdUpdates != nil {
			*sd.sdUpdates <- tg
		}
	}

	if sd.sdUpdates != nil {
		close(*sd.sdUpdates)
	}
}

// Run implements the TargetProvider interface.
func (sd *ServersetDiscovery) Run(ch chan<- *config.TargetGroup, done <-chan struct{}) {
	// Send on everything we have seen so far.
	sd.mu.Lock()
	for _, targetGroup := range sd.sources {
		ch <- targetGroup
	}
	// Tell processUpdates to send future updates.
	sd.sdUpdates = &ch
	sd.mu.Unlock()

	<-done
	sd.treeCache.Stop()
}

func parseServersetMember(data []byte, path string) (*clientmodel.LabelSet, error) {
	member := serversetMember{}
	err := json.Unmarshal(data, &member)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling serverset member %q: %s", path, err)
	}

	labels := clientmodel.LabelSet{}
	labels[serversetPathLabel] = clientmodel.LabelValue(path)
	labels[clientmodel.AddressLabel] = clientmodel.LabelValue(
		fmt.Sprintf("%s:%d", member.ServiceEndpoint.Host, member.ServiceEndpoint.Port))

	labels[serversetEndpointLabelPrefix+"_host"] = clientmodel.LabelValue(member.ServiceEndpoint.Host)
	labels[serversetEndpointLabelPrefix+"_port"] = clientmodel.LabelValue(fmt.Sprintf("%d", member.ServiceEndpoint.Port))

	for name, endpoint := range member.AdditionalEndpoints {
		cleanName := clientmodel.LabelName(strutil.SanitizeLabelName(name))
		labels[serversetEndpointLabelPrefix+"_host_"+cleanName] = clientmodel.LabelValue(
			endpoint.Host)
		labels[serversetEndpointLabelPrefix+"_port_"+cleanName] = clientmodel.LabelValue(
			fmt.Sprintf("%d", endpoint.Port))

	}

	labels[serversetStatusLabel] = clientmodel.LabelValue(member.Status)

	return &labels, nil
}

type zookeeperTreeCache struct {
	conn     *zk.Conn
	prefix   string
	events   chan zookeeperTreeCacheEvent
	zkEvents chan zk.Event
	stop     chan struct{}
	head     *zookeeperTreeCacheNode
}

type zookeeperTreeCacheEvent struct {
	Path string
	Data *[]byte
}

type zookeeperTreeCacheNode struct {
	data     *[]byte
	events   chan zk.Event
	done     chan struct{}
	stopped  bool
	children map[string]*zookeeperTreeCacheNode
}

func NewZookeeperTreeCache(conn *zk.Conn, path string, events chan zookeeperTreeCacheEvent) *zookeeperTreeCache {
	tc := &zookeeperTreeCache{
		conn:   conn,
		prefix: path,
		events: events,
		stop:   make(chan struct{}),
	}
	tc.head = &zookeeperTreeCacheNode{
		events:   make(chan zk.Event),
		children: map[string]*zookeeperTreeCacheNode{},
		stopped:  true,
	}
	err := tc.recursiveNodeUpdate(path, tc.head)
	if err != nil {
		log.Errorf("Error during initial read of Zookeeper: %s", err)
	}
	go tc.loop(err != nil)
	return tc
}

func (tc *zookeeperTreeCache) Stop() {
	tc.stop <- struct{}{}
}

func (tc *zookeeperTreeCache) loop(failureMode bool) {
	retryChan := make(chan struct{})

	failure := func() {
		failureMode = true
		time.AfterFunc(time.Second*10, func() {
			retryChan <- struct{}{}
		})
	}
	if failureMode {
		failure()
	}

	for {
		select {
		case ev := <-tc.head.events:
			log.Debugf("Received Zookeeper event: %s", ev)
			if failureMode {
				continue
			}
			if ev.Type == zk.EventNotWatching {
				log.Infof("Lost connection to Zookeeper.")
				failure()
			} else {
				path := strings.TrimPrefix(ev.Path, tc.prefix)
				parts := strings.Split(path, "/")
				node := tc.head
				for _, part := range parts[1:] {
					childNode := node.children[part]
					if childNode == nil {
						childNode = &zookeeperTreeCacheNode{
							events:   tc.head.events,
							children: map[string]*zookeeperTreeCacheNode{},
							done:     make(chan struct{}, 1),
						}
						node.children[part] = childNode
					}
					node = childNode
				}
				err := tc.recursiveNodeUpdate(ev.Path, node)
				if err != nil {
					log.Errorf("Error during processing of Zookeeper event: %s", err)
					failure()
				} else if tc.head.data == nil {
					log.Errorf("Error during processing of Zookeeper event: path %s no longer exists", tc.prefix)
					failure()
				}
			}
		case <-retryChan:
			log.Infof("Attempting to resync state with Zookeeper")
			err := tc.recursiveNodeUpdate(tc.prefix, tc.head)
			if err != nil {
				log.Errorf("Error during Zookeeper resync: %s", err)
				failure()
			} else {
				log.Infof("Zookeeper resync successful")
				failureMode = false
			}
		case <-tc.stop:
			close(tc.events)
			return
		}
	}
}

func (tc *zookeeperTreeCache) recursiveNodeUpdate(path string, node *zookeeperTreeCacheNode) error {
	data, _, dataWatcher, err := tc.conn.GetW(path)
	if err == zk.ErrNoNode {
		tc.recursiveDelete(path, node)
		if node == tc.head {
			return fmt.Errorf("path %s does not exist", path)
		}
		return nil
	} else if err != nil {
		return err
	}

	if node.data == nil || !bytes.Equal(*node.data, data) {
		node.data = &data
		tc.events <- zookeeperTreeCacheEvent{Path: path, Data: node.data}
	}

	children, _, childWatcher, err := tc.conn.ChildrenW(path)
	if err == zk.ErrNoNode {
		tc.recursiveDelete(path, node)
		return nil
	} else if err != nil {
		return err
	}

	currentChildren := map[string]struct{}{}
	for _, child := range children {
		currentChildren[child] = struct{}{}
		childNode := node.children[child]
		// Does not already exists, create it.
		if childNode == nil {
			node.children[child] = &zookeeperTreeCacheNode{
				events:   node.events,
				children: map[string]*zookeeperTreeCacheNode{},
				done:     make(chan struct{}, 1),
			}
		}
		err = tc.recursiveNodeUpdate(path+"/"+child, node.children[child])
		if err != nil {
			return err
		}
	}
	// Remove nodes that no longer exist
	for name, childNode := range node.children {
		if _, ok := currentChildren[name]; !ok || node.data == nil {
			tc.recursiveDelete(path+"/"+name, childNode)
			delete(node.children, name)
		}
	}

	go func() {
		// Pass up zookeeper events, until the node is deleted.
		select {
		case event := <-dataWatcher:
			node.events <- event
		case event := <-childWatcher:
			node.events <- event
		case <-node.done:
		}
	}()
	return nil
}

func (tc *zookeeperTreeCache) recursiveDelete(path string, node *zookeeperTreeCacheNode) {
	if !node.stopped {
		node.done <- struct{}{}
		node.stopped = true
	}
	if node.data != nil {
		tc.events <- zookeeperTreeCacheEvent{Path: path, Data: nil}
		node.data = nil
	}
	for name, childNode := range node.children {
		tc.recursiveDelete(path+"/"+name, childNode)
	}
}
