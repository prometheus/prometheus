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
	"fmt"
	"net"
	"strconv"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/net/context"
	"k8s.io/client-go/1.5/pkg/api"
	apiv1 "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/cache"
)

// Node discovers Kubernetes nodes.
type Node struct {
	logger   log.Logger
	informer cache.SharedInformer
	store    cache.Store
}

// NewNode returns a new node discovery.
func NewNode(l log.Logger, inf cache.SharedInformer) *Node {
	return &Node{logger: l, informer: inf, store: inf.GetStore()}
}

// Run implements the TargetProvider interface.
func (n *Node) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Send full initial set of pod targets.
	var initial []*config.TargetGroup
	for _, o := range n.store.List() {
		tg := n.buildNode(o.(*apiv1.Node))
		initial = append(initial, tg)
	}
	select {
	case <-ctx.Done():
		return
	case ch <- initial:
	}

	// Send target groups for service updates.
	send := func(tg *config.TargetGroup) {
		select {
		case <-ctx.Done():
		case ch <- []*config.TargetGroup{tg}:
		}
	}
	n.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("node", "add").Inc()

			node, err := convertToNode(o)
			if err != nil {
				n.logger.With("err", err).Errorln("converting to Node object failed")
				return
			}
			send(n.buildNode(node))
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("node", "delete").Inc()

			node, err := convertToNode(o)
			if err != nil {
				n.logger.With("err", err).Errorln("converting to Node object failed")
				return
			}
			send(&config.TargetGroup{Source: nodeSource(node)})
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("node", "update").Inc()

			node, err := convertToNode(o)
			if err != nil {
				n.logger.With("err", err).Errorln("converting to Node object failed")
				return
			}
			send(n.buildNode(node))
		},
	})

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func convertToNode(o interface{}) (*apiv1.Node, error) {
	node, isNode := o.(*apiv1.Node)
	if !isNode {
		deletedState, ok := o.(cache.DeletedFinalStateUnknown)
		if !ok {
			return nil, fmt.Errorf("Received unexpected object: %v", o)
		}
		node, ok = deletedState.Obj.(*apiv1.Node)
		if !ok {
			return nil, fmt.Errorf("DeletedFinalStateUnknown contained non-Node object: %v", deletedState.Obj)
		}
	}

	return node, nil
}

func nodeSource(n *apiv1.Node) string {
	return "node/" + n.Name
}

const (
	nodeNameLabel        = metaLabelPrefix + "node_name"
	nodeLabelPrefix      = metaLabelPrefix + "node_label_"
	nodeAnnotationPrefix = metaLabelPrefix + "node_annotation_"
	nodeAddressPrefix    = metaLabelPrefix + "node_address_"
)

func nodeLabels(n *apiv1.Node) model.LabelSet {
	ls := make(model.LabelSet, len(n.Labels)+len(n.Annotations)+2)

	ls[nodeNameLabel] = lv(n.Name)

	for k, v := range n.Labels {
		ln := strutil.SanitizeLabelName(nodeLabelPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
	}

	for k, v := range n.Annotations {
		ln := strutil.SanitizeLabelName(nodeAnnotationPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
	}
	return ls
}

func (n *Node) buildNode(node *apiv1.Node) *config.TargetGroup {
	tg := &config.TargetGroup{
		Source: nodeSource(node),
	}
	tg.Labels = nodeLabels(node)

	addr, addrMap, err := nodeAddress(node)
	if err != nil {
		n.logger.With("err", err).Debugf("No node address found")
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

// nodeAddresses returns the provided node's address, based on the priority:
// 1. NodeInternalIP
// 2. NodeExternalIP
// 3. NodeLegacyHostIP
// 3. NodeHostName
//
// Derived from k8s.io/kubernetes/pkg/util/node/node.go
func nodeAddress(node *apiv1.Node) (string, map[apiv1.NodeAddressType][]string, error) {
	m := map[apiv1.NodeAddressType][]string{}
	for _, a := range node.Status.Addresses {
		m[a.Type] = append(m[a.Type], a.Address)
	}

	if addresses, ok := m[apiv1.NodeInternalIP]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeExternalIP]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeAddressType(api.NodeLegacyHostIP)]; ok {
		return addresses[0], m, nil
	}
	if addresses, ok := m[apiv1.NodeHostName]; ok {
		return addresses[0], m, nil
	}
	return "", m, fmt.Errorf("host address unknown")
}
