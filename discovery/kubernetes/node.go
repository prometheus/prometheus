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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	NodeLegacyHostIP = "LegacyHostIP"
)

// Node discovers Kubernetes nodes.
type Node struct {
	logger   *slog.Logger
	informer cache.SharedInformer
	store    cache.Store
	queue    *workqueue.Type
}

// NewNode returns a new node discovery.
func NewNode(l *slog.Logger, inf cache.SharedInformer, eventCount *prometheus.CounterVec) *Node {
	if l == nil {
		l = promslog.NewNopLogger()
	}

	nodeAddCount := eventCount.WithLabelValues(RoleNode.String(), MetricLabelRoleAdd)
	nodeUpdateCount := eventCount.WithLabelValues(RoleNode.String(), MetricLabelRoleUpdate)
	nodeDeleteCount := eventCount.WithLabelValues(RoleNode.String(), MetricLabelRoleDelete)

	n := &Node{
		logger:   l,
		informer: inf,
		store:    inf.GetStore(),
		queue:    workqueue.NewNamed(RoleNode.String()),
	}

	_, err := n.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			nodeAddCount.Inc()
			n.enqueue(o)
		},
		DeleteFunc: func(o interface{}) {
			nodeDeleteCount.Inc()
			n.enqueue(o)
		},
		UpdateFunc: func(_, o interface{}) {
			nodeUpdateCount.Inc()
			n.enqueue(o)
		},
	})
	if err != nil {
		l.Error("Error adding nodes event handler.", "err", err)
	}
	return n
}

func (n *Node) enqueue(obj interface{}) {
	key, err := nodeName(obj)
	if err != nil {
		return
	}

	n.queue.Add(key)
}

// Run implements the Discoverer interface.
func (n *Node) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer n.queue.ShutDown()

	if !cache.WaitForCacheSync(ctx.Done(), n.informer.HasSynced) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			n.logger.Error("node informer unable to sync cache")
		}
		return
	}

	go func() {
		for n.process(ctx, ch) {
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (n *Node) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	keyObj, quit := n.queue.Get()
	if quit {
		return false
	}
	defer n.queue.Done(keyObj)
	key := keyObj.(string)

	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return true
	}

	o, exists, err := n.store.GetByKey(key)
	if err != nil {
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: nodeSourceFromName(name)})
		return true
	}
	node, err := convertToNode(o)
	if err != nil {
		n.logger.Error("converting to Node object failed", "err", err)
		return true
	}
	send(ctx, ch, n.buildNode(node))
	return true
}

func convertToNode(o interface{}) (*apiv1.Node, error) {
	node, ok := o.(*apiv1.Node)
	if ok {
		return node, nil
	}

	return nil, fmt.Errorf("received unexpected object: %v", o)
}

func nodeSource(n *apiv1.Node) string {
	return nodeSourceFromName(n.Name)
}

func nodeSourceFromName(name string) string {
	return "node/" + name
}

const (
	nodeProviderIDLabel = metaLabelPrefix + "node_provider_id"
	nodeAddressPrefix   = metaLabelPrefix + "node_address_"
)

func nodeLabels(n *apiv1.Node) model.LabelSet {
	// Each label and annotation will create two key-value pairs in the map.
	ls := make(model.LabelSet)

	ls[nodeProviderIDLabel] = lv(n.Spec.ProviderID)

	addObjectMetaLabels(ls, n.ObjectMeta, RoleNode)

	return ls
}

func (n *Node) buildNode(node *apiv1.Node) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: nodeSource(node),
	}
	tg.Labels = nodeLabels(node)

	addr, addrMap, err := nodeAddress(node)
	if err != nil {
		n.logger.Warn("No node address found", "err", err)
		return nil
	}
	addr = net.JoinHostPort(addr, strconv.FormatInt(int64(node.Status.DaemonEndpoints.KubeletEndpoint.Port), 10))

	t := model.LabelSet{
		model.AddressLabel:  lv(addr),
		model.InstanceLabel: lv(node.Name),
	}

	for ty, a := range addrMap {
		ln := strutil.SanitizeLabelName(nodeAddressPrefix + string(ty))
		t[model.LabelName(ln)] = lv(a[0])
	}
	tg.Targets = append(tg.Targets, t)

	return tg
}

// nodeAddress returns the provided node's address, based on the priority:
// 1. NodeInternalIP
// 2. NodeInternalDNS
// 3. NodeExternalIP
// 4. NodeExternalDNS
// 5. NodeLegacyHostIP
// 6. NodeHostName
//
// Derived from k8s.io/kubernetes/pkg/util/node/node.go.
func nodeAddress(node *apiv1.Node) (string, map[apiv1.NodeAddressType][]string, error) {
	m := map[apiv1.NodeAddressType][]string{}
	for _, a := range node.Status.Addresses {
		m[a.Type] = append(m[a.Type], a.Address)
	}

	if addresses, ok := m[apiv1.NodeInternalIP]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeInternalDNS]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeExternalIP]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeExternalDNS]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeAddressType(NodeLegacyHostIP)]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeHostName]; ok {
		return addresses[0], m, nil
	}
	return "", m, errors.New("host address unknown")
}
