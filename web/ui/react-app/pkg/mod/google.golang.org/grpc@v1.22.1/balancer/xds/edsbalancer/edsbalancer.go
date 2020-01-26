/*
 * Copyright 2019 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package edsbalancer implements a balancer to handle EDS responses.
package edsbalancer

import (
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strconv"
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/balancer/xds/internal"
	edspb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/eds"
	endpointpb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/endpoint/endpoint"
	percentpb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/type/percent"
	"google.golang.org/grpc/balancer/xds/lrs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

type localityConfig struct {
	weight uint32
	addrs  []resolver.Address
}

// EDSBalancer does load balancing based on the EDS responses. Note that it
// doesn't implement the balancer interface. It's intended to be used by a high
// level balancer implementation.
//
// The localities are picked as weighted round robin. A configurable child
// policy is used to manage endpoints in each locality.
type EDSBalancer struct {
	balancer.ClientConn

	bg                 *balancerGroup
	subBalancerBuilder balancer.Builder
	lidToConfig        map[internal.Locality]*localityConfig
	loadStore          lrs.Store

	pickerMu    sync.Mutex
	drops       []*dropper
	innerPicker balancer.Picker    // The picker without drop support.
	innerState  connectivity.State // The state of the picker.
}

// NewXDSBalancer create a new EDSBalancer.
func NewXDSBalancer(cc balancer.ClientConn, loadStore lrs.Store) *EDSBalancer {
	xdsB := &EDSBalancer{
		ClientConn:         cc,
		subBalancerBuilder: balancer.Get(roundrobin.Name),

		lidToConfig: make(map[internal.Locality]*localityConfig),
		loadStore:   loadStore,
	}
	// Don't start balancer group here. Start it when handling the first EDS
	// response. Otherwise the balancer group will be started with round-robin,
	// and if users specify a different sub-balancer, all balancers in balancer
	// group will be closed and recreated when sub-balancer update happens.
	return xdsB
}

// HandleChildPolicy updates the child balancers handling endpoints. Child
// policy is roundrobin by default. If the specified balancer is not installed,
// the old child balancer will be used.
//
// HandleChildPolicy and HandleEDSResponse must be called by the same goroutine.
func (xdsB *EDSBalancer) HandleChildPolicy(name string, config json.RawMessage) {
	// name could come from cdsResp.GetLbPolicy().String(). LbPolicy.String()
	// are all UPPER_CASE with underscore.
	//
	// No conversion is needed here because balancer package converts all names
	// into lower_case before registering/looking up.
	xdsB.updateSubBalancerName(name)
	// TODO: (eds) send balancer config to the new child balancers.
}

func (xdsB *EDSBalancer) updateSubBalancerName(subBalancerName string) {
	if xdsB.subBalancerBuilder.Name() == subBalancerName {
		return
	}
	newSubBalancerBuilder := balancer.Get(subBalancerName)
	if newSubBalancerBuilder == nil {
		grpclog.Infof("EDSBalancer: failed to find balancer with name %q, keep using %q", subBalancerName, xdsB.subBalancerBuilder.Name())
		return
	}
	xdsB.subBalancerBuilder = newSubBalancerBuilder
	if xdsB.bg != nil {
		// xdsB.bg == nil until the first EDS response is handled. There's no
		// need to update balancer group before that.
		for id, config := range xdsB.lidToConfig {
			// TODO: (eds) add support to balancer group to support smoothly
			//  switching sub-balancers (keep old balancer around until new
			//  balancer becomes ready).
			xdsB.bg.remove(id)
			xdsB.bg.add(id, config.weight, xdsB.subBalancerBuilder)
			xdsB.bg.handleResolvedAddrs(id, config.addrs)
		}
	}
}

// updateDrops compares new drop policies with the old. If they are different,
// it updates the drop policies and send ClientConn an updated picker.
func (xdsB *EDSBalancer) updateDrops(dropPolicies []*edspb.ClusterLoadAssignment_Policy_DropOverload) {
	var (
		newDrops     []*dropper
		dropsChanged bool
	)
	for i, dropPolicy := range dropPolicies {
		percentage := dropPolicy.GetDropPercentage()
		var (
			numerator   = percentage.GetNumerator()
			denominator uint32
		)
		switch percentage.GetDenominator() {
		case percentpb.FractionalPercent_HUNDRED:
			denominator = 100
		case percentpb.FractionalPercent_TEN_THOUSAND:
			denominator = 10000
		case percentpb.FractionalPercent_MILLION:
			denominator = 1000000
		}
		newDrops = append(newDrops, newDropper(numerator, denominator, dropPolicy.GetCategory()))

		// The following reading xdsB.drops doesn't need mutex because it can only
		// be updated by the code following.
		if dropsChanged {
			continue
		}
		if i >= len(xdsB.drops) {
			dropsChanged = true
			continue
		}
		if oldDrop := xdsB.drops[i]; numerator != oldDrop.numerator || denominator != oldDrop.denominator {
			dropsChanged = true
		}
	}
	if dropsChanged {
		xdsB.pickerMu.Lock()
		xdsB.drops = newDrops
		if xdsB.innerPicker != nil {
			// Update picker with old inner picker, new drops.
			xdsB.ClientConn.UpdateBalancerState(xdsB.innerState, newDropPicker(xdsB.innerPicker, newDrops, xdsB.loadStore))
		}
		xdsB.pickerMu.Unlock()
	}
}

// HandleEDSResponse handles the EDS response and creates/deletes localities and
// SubConns. It also handles drops.
//
// HandleCDSResponse and HandleEDSResponse must be called by the same goroutine.
func (xdsB *EDSBalancer) HandleEDSResponse(edsResp *edspb.ClusterLoadAssignment) {
	// Create balancer group if it's never created (this is the first EDS
	// response).
	if xdsB.bg == nil {
		xdsB.bg = newBalancerGroup(xdsB, xdsB.loadStore)
	}

	// TODO: Unhandled fields from EDS response:
	//  - edsResp.GetPolicy().GetOverprovisioningFactor()
	//  - locality.GetPriority()
	//  - lbEndpoint.GetMetadata(): contains BNS name, send to sub-balancers
	//    - as service config or as resolved address
	//  - if socketAddress is not ip:port
	//     - socketAddress.GetNamedPort(), socketAddress.GetResolverName()
	//     - resolve endpoint's name with another resolver

	xdsB.updateDrops(edsResp.GetPolicy().GetDropOverloads())

	// Filter out all localities with weight 0.
	//
	// Locality weighted load balancer can be enabled by setting an option in
	// CDS, and the weight of each locality. Currently, without the guarantee
	// that CDS is always sent, we assume locality weighted load balance is
	// always enabled, and ignore all weight 0 localities.
	//
	// In the future, we should look at the config in CDS response and decide
	// whether locality weight matters.
	newEndpoints := make([]*endpointpb.LocalityLbEndpoints, 0, len(edsResp.Endpoints))
	for _, locality := range edsResp.Endpoints {
		if locality.GetLoadBalancingWeight().GetValue() == 0 {
			continue
		}
		newEndpoints = append(newEndpoints, locality)
	}

	// newLocalitiesSet contains all names of localitis in the new EDS response.
	// It's used to delete localities that are removed in the new EDS response.
	newLocalitiesSet := make(map[internal.Locality]struct{})
	for _, locality := range newEndpoints {
		// One balancer for each locality.

		l := locality.GetLocality()
		if l == nil {
			grpclog.Warningf("xds: received LocalityLbEndpoints with <nil> Locality")
			continue
		}
		lid := internal.Locality{
			Region:  l.Region,
			Zone:    l.Zone,
			SubZone: l.SubZone,
		}
		newLocalitiesSet[lid] = struct{}{}

		newWeight := locality.GetLoadBalancingWeight().GetValue()
		var newAddrs []resolver.Address
		for _, lbEndpoint := range locality.GetLbEndpoints() {
			socketAddress := lbEndpoint.GetEndpoint().GetAddress().GetSocketAddress()
			newAddrs = append(newAddrs, resolver.Address{
				Addr: net.JoinHostPort(socketAddress.GetAddress(), strconv.Itoa(int(socketAddress.GetPortValue()))),
			})
		}
		var weightChanged, addrsChanged bool
		config, ok := xdsB.lidToConfig[lid]
		if !ok {
			// A new balancer, add it to balancer group and balancer map.
			xdsB.bg.add(lid, newWeight, xdsB.subBalancerBuilder)
			config = &localityConfig{
				weight: newWeight,
			}
			xdsB.lidToConfig[lid] = config

			// weightChanged is false for new locality, because there's no need to
			// update weight in bg.
			addrsChanged = true
		} else {
			// Compare weight and addrs.
			if config.weight != newWeight {
				weightChanged = true
			}
			if !reflect.DeepEqual(config.addrs, newAddrs) {
				addrsChanged = true
			}
		}

		if weightChanged {
			config.weight = newWeight
			xdsB.bg.changeWeight(lid, newWeight)
		}

		if addrsChanged {
			config.addrs = newAddrs
			xdsB.bg.handleResolvedAddrs(lid, newAddrs)
		}
	}

	// Delete localities that are removed in the latest response.
	for lid := range xdsB.lidToConfig {
		if _, ok := newLocalitiesSet[lid]; !ok {
			xdsB.bg.remove(lid)
			delete(xdsB.lidToConfig, lid)
		}
	}
}

// HandleSubConnStateChange handles the state change and update pickers accordingly.
func (xdsB *EDSBalancer) HandleSubConnStateChange(sc balancer.SubConn, s connectivity.State) {
	xdsB.bg.handleSubConnStateChange(sc, s)
}

// UpdateBalancerState overrides balancer.ClientConn to wrap the picker in a
// dropPicker.
func (xdsB *EDSBalancer) UpdateBalancerState(s connectivity.State, p balancer.Picker) {
	xdsB.pickerMu.Lock()
	defer xdsB.pickerMu.Unlock()
	xdsB.innerPicker = p
	xdsB.innerState = s
	// Don't reset drops when it's a state change.
	xdsB.ClientConn.UpdateBalancerState(s, newDropPicker(p, xdsB.drops, xdsB.loadStore))
}

// Close closes the balancer.
func (xdsB *EDSBalancer) Close() {
	if xdsB.bg != nil {
		xdsB.bg.close()
	}
}

type dropPicker struct {
	drops     []*dropper
	p         balancer.Picker
	loadStore lrs.Store
}

func newDropPicker(p balancer.Picker, drops []*dropper, loadStore lrs.Store) *dropPicker {
	return &dropPicker{
		drops:     drops,
		p:         p,
		loadStore: loadStore,
	}
}

func (d *dropPicker) Pick(ctx context.Context, opts balancer.PickOptions) (conn balancer.SubConn, done func(balancer.DoneInfo), err error) {
	var (
		drop     bool
		category string
	)
	for _, dp := range d.drops {
		if dp.drop() {
			drop = true
			category = dp.category
			break
		}
	}
	if drop {
		if d.loadStore != nil {
			d.loadStore.CallDropped(category)
		}
		return nil, nil, status.Errorf(codes.Unavailable, "RPC is dropped")
	}
	// TODO: (eds) don't drop unless the inner picker is READY. Similar to
	// https://github.com/grpc/grpc-go/issues/2622.
	return d.p.Pick(ctx, opts)
}
