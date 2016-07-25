// Copyright 2016 The Prometheus Authors
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

package kubernetes

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/net/context"
)

type nodeDiscovery struct {
	mtx           sync.RWMutex
	nodes         map[string]*Node
	retryInterval time.Duration
	kd            *Discovery
}

func (d *nodeDiscovery) run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	select {
	case ch <- []*config.TargetGroup{d.updateNodesTargetGroup()}:
	case <-ctx.Done():
		return
	}

	update := make(chan *nodeEvent, 10)
	go d.watchNodes(update, ctx.Done(), d.retryInterval)

	for {
		tgs := []*config.TargetGroup{}
		select {
		case <-ctx.Done():
			return
		case e := <-update:
			log.Debugf("k8s discovery received node event (EventType=%s, Node Name=%s)", e.EventType, e.Node.ObjectMeta.Name)
			d.updateNode(e.Node, e.EventType)
			tgs = append(tgs, d.updateNodesTargetGroup())
		}
		if tgs == nil {
			continue
		}

		for _, tg := range tgs {
			select {
			case ch <- []*config.TargetGroup{tg}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (d *nodeDiscovery) updateNodesTargetGroup() *config.TargetGroup {
	d.mtx.RLock()
	defer d.mtx.RUnlock()

	tg := &config.TargetGroup{
		Source: nodesTargetGroupName,
		Labels: model.LabelSet{
			roleLabel: model.LabelValue("node"),
		},
	}

	// Now let's loop through the nodes & add them to the target group with appropriate labels.
	for nodeName, node := range d.nodes {
		defaultNodeAddress, nodeAddressMap, err := nodeAddresses(node)
		if err != nil {
			log.Debugf("Skipping node %s: %s", node.Name, err)
			continue
		}

		kubeletPort := int(node.Status.DaemonEndpoints.KubeletEndpoint.Port)

		address := fmt.Sprintf("%s:%d", defaultNodeAddress.String(), kubeletPort)

		t := model.LabelSet{
			model.AddressLabel:  model.LabelValue(address),
			model.InstanceLabel: model.LabelValue(nodeName),
		}

		for addrType, ip := range nodeAddressMap {
			labelName := strutil.SanitizeLabelName(nodeAddressPrefix + string(addrType))
			t[model.LabelName(labelName)] = model.LabelValue(ip[0].String())
		}

		t[model.LabelName(nodePortLabel)] = model.LabelValue(strconv.Itoa(kubeletPort))

		for k, v := range node.ObjectMeta.Labels {
			labelName := strutil.SanitizeLabelName(nodeLabelPrefix + k)
			t[model.LabelName(labelName)] = model.LabelValue(v)
		}
		tg.Targets = append(tg.Targets, t)
	}

	return tg
}

// watchNodes watches nodes as they come & go.
func (d *nodeDiscovery) watchNodes(events chan *nodeEvent, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		nodes, resourceVersion, err := d.getNodes()
		if err != nil {
			log.Errorf("Cannot initialize nodes collection: %s", err)
			return
		}

		// Reset the known nodes.
		d.mtx.Lock()
		d.nodes = map[string]*Node{}
		d.mtx.Unlock()

		for _, node := range nodes {
			events <- &nodeEvent{Added, node}
		}

		req, err := http.NewRequest("GET", nodesURL, nil)
		if err != nil {
			log.Errorf("Cannot create nodes request: %s", err)
			return
		}
		values := req.URL.Query()
		values.Add("watch", "true")
		values.Add("resourceVersion", resourceVersion)
		req.URL.RawQuery = values.Encode()
		res, err := d.kd.queryAPIServerReq(req)
		if err != nil {
			log.Errorf("Failed to watch nodes: %s", err)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			log.Errorf("Failed to watch nodes: %d", res.StatusCode)
			return
		}

		d := json.NewDecoder(res.Body)

		for {
			var event nodeEvent
			if err := d.Decode(&event); err != nil {
				log.Errorf("Watch nodes unexpectedly closed: %s", err)
				return
			}

			select {
			case events <- &event:
			case <-done:
			}
		}
	}, retryInterval, done)
}

func (d *nodeDiscovery) updateNode(node *Node, eventType EventType) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	updatedNodeName := node.ObjectMeta.Name
	switch eventType {
	case Deleted:
		// Deleted - remove from nodes map.
		delete(d.nodes, updatedNodeName)
	case Added, Modified:
		// Added/Modified - update the node in the nodes map.
		d.nodes[updatedNodeName] = node
	}
}

func (d *nodeDiscovery) getNodes() (map[string]*Node, string, error) {
	res, err := d.kd.queryAPIServerPath(nodesURL)
	if err != nil {
		// If we can't list nodes then we can't watch them. Assume this is a misconfiguration
		// & return error.
		return nil, "", fmt.Errorf("unable to list Kubernetes nodes: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unable to list Kubernetes nodes; unexpected response: %d %s", res.StatusCode, res.Status)
	}

	var nodes NodeList
	if err := json.NewDecoder(res.Body).Decode(&nodes); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, "", fmt.Errorf("unable to list Kubernetes nodes; unexpected response body: %s", string(body))
	}

	nodeMap := map[string]*Node{}
	for idx, node := range nodes.Items {
		nodeMap[node.ObjectMeta.Name] = &nodes.Items[idx]
	}

	return nodeMap, nodes.ResourceVersion, nil
}

// nodeAddresses returns the provided node's address, based on the priority:
// 1. NodeInternalIP
// 2. NodeExternalIP
// 3. NodeLegacyHostIP
//
// Copied from k8s.io/kubernetes/pkg/util/node/node.go
func nodeAddresses(node *Node) (net.IP, map[NodeAddressType][]net.IP, error) {
	addresses := node.Status.Addresses
	addressMap := map[NodeAddressType][]net.IP{}
	for _, addr := range addresses {
		ip := net.ParseIP(addr.Address)
		// All addresses should be valid IPs.
		if ip == nil {
			continue
		}
		addressMap[addr.Type] = append(addressMap[addr.Type], ip)
	}
	if addresses, ok := addressMap[NodeInternalIP]; ok {
		return addresses[0], addressMap, nil
	}
	if addresses, ok := addressMap[NodeExternalIP]; ok {
		return addresses[0], addressMap, nil
	}
	if addresses, ok := addressMap[NodeLegacyHostIP]; ok {
		return addresses[0], addressMap, nil
	}
	return nil, nil, fmt.Errorf("host IP unknown; known addresses: %v", addresses)
}
