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

package edsbalancer

import (
	"context"
	"reflect"
	"testing"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/balancer/xds/internal"
	orcapb "google.golang.org/grpc/balancer/xds/internal/proto/udpa/data/orca/v1/orca_load_report"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

var (
	rrBuilder        = balancer.Get(roundrobin.Name)
	testBalancerIDs  = []internal.Locality{{Region: "b1"}, {Region: "b2"}, {Region: "b3"}}
	testBackendAddrs = []resolver.Address{{Addr: "1.1.1.1:1"}, {Addr: "2.2.2.2:2"}, {Addr: "3.3.3.3:3"}, {Addr: "4.4.4.4:4"}}
)

// 1 balancer, 1 backend -> 2 backends -> 1 backend.
func TestBalancerGroup_OneRR_AddRemoveBackend(t *testing.T) {
	cc := newTestClientConn(t)
	bg := newBalancerGroup(cc, nil)

	// Add one balancer to group.
	bg.add(testBalancerIDs[0], 1, rrBuilder)
	// Send one resolved address.
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:1])

	// Send subconn state change.
	sc1 := <-cc.newSubConnCh
	bg.handleSubConnStateChange(sc1, connectivity.Connecting)
	bg.handleSubConnStateChange(sc1, connectivity.Ready)

	// Test pick with one backend.
	p1 := <-cc.newPickerCh
	for i := 0; i < 5; i++ {
		gotSC, _, _ := p1.Pick(context.Background(), balancer.PickOptions{})
		if !reflect.DeepEqual(gotSC, sc1) {
			t.Fatalf("picker.Pick, got %v, want %v", gotSC, sc1)
		}
	}

	// Send two addresses.
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:2])
	// Expect one new subconn, send state update.
	sc2 := <-cc.newSubConnCh
	bg.handleSubConnStateChange(sc2, connectivity.Connecting)
	bg.handleSubConnStateChange(sc2, connectivity.Ready)

	// Test roundrobin pick.
	p2 := <-cc.newPickerCh
	want := []balancer.SubConn{sc1, sc2}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p2.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	// Remove the first address.
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[1:2])
	scToRemove := <-cc.removeSubConnCh
	if !reflect.DeepEqual(scToRemove, sc1) {
		t.Fatalf("RemoveSubConn, want %v, got %v", sc1, scToRemove)
	}
	bg.handleSubConnStateChange(scToRemove, connectivity.Shutdown)

	// Test pick with only the second subconn.
	p3 := <-cc.newPickerCh
	for i := 0; i < 5; i++ {
		gotSC, _, _ := p3.Pick(context.Background(), balancer.PickOptions{})
		if !reflect.DeepEqual(gotSC, sc2) {
			t.Fatalf("picker.Pick, got %v, want %v", gotSC, sc2)
		}
	}
}

// 2 balancers, each with 1 backend.
func TestBalancerGroup_TwoRR_OneBackend(t *testing.T) {
	cc := newTestClientConn(t)
	bg := newBalancerGroup(cc, nil)

	// Add two balancers to group and send one resolved address to both
	// balancers.
	bg.add(testBalancerIDs[0], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:1])
	sc1 := <-cc.newSubConnCh

	bg.add(testBalancerIDs[1], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[1], testBackendAddrs[0:1])
	sc2 := <-cc.newSubConnCh

	// Send state changes for both subconns.
	bg.handleSubConnStateChange(sc1, connectivity.Connecting)
	bg.handleSubConnStateChange(sc1, connectivity.Ready)
	bg.handleSubConnStateChange(sc2, connectivity.Connecting)
	bg.handleSubConnStateChange(sc2, connectivity.Ready)

	// Test roundrobin on the last picker.
	p1 := <-cc.newPickerCh
	want := []balancer.SubConn{sc1, sc2}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p1.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}
}

// 2 balancers, each with more than 1 backends.
func TestBalancerGroup_TwoRR_MoreBackends(t *testing.T) {
	cc := newTestClientConn(t)
	bg := newBalancerGroup(cc, nil)

	// Add two balancers to group and send one resolved address to both
	// balancers.
	bg.add(testBalancerIDs[0], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:2])
	sc1 := <-cc.newSubConnCh
	sc2 := <-cc.newSubConnCh

	bg.add(testBalancerIDs[1], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[1], testBackendAddrs[2:4])
	sc3 := <-cc.newSubConnCh
	sc4 := <-cc.newSubConnCh

	// Send state changes for both subconns.
	bg.handleSubConnStateChange(sc1, connectivity.Connecting)
	bg.handleSubConnStateChange(sc1, connectivity.Ready)
	bg.handleSubConnStateChange(sc2, connectivity.Connecting)
	bg.handleSubConnStateChange(sc2, connectivity.Ready)
	bg.handleSubConnStateChange(sc3, connectivity.Connecting)
	bg.handleSubConnStateChange(sc3, connectivity.Ready)
	bg.handleSubConnStateChange(sc4, connectivity.Connecting)
	bg.handleSubConnStateChange(sc4, connectivity.Ready)

	// Test roundrobin on the last picker.
	p1 := <-cc.newPickerCh
	want := []balancer.SubConn{sc1, sc2, sc3, sc4}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p1.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	// Turn sc2's connection down, should be RR between balancers.
	bg.handleSubConnStateChange(sc2, connectivity.TransientFailure)
	p2 := <-cc.newPickerCh
	// Expect two sc1's in the result, because balancer1 will be picked twice,
	// but there's only one sc in it.
	want = []balancer.SubConn{sc1, sc1, sc3, sc4}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p2.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	// Remove sc3's addresses.
	bg.handleResolvedAddrs(testBalancerIDs[1], testBackendAddrs[3:4])
	scToRemove := <-cc.removeSubConnCh
	if !reflect.DeepEqual(scToRemove, sc3) {
		t.Fatalf("RemoveSubConn, want %v, got %v", sc3, scToRemove)
	}
	bg.handleSubConnStateChange(scToRemove, connectivity.Shutdown)
	p3 := <-cc.newPickerCh
	want = []balancer.SubConn{sc1, sc4}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p3.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	// Turn sc1's connection down.
	bg.handleSubConnStateChange(sc1, connectivity.TransientFailure)
	p4 := <-cc.newPickerCh
	want = []balancer.SubConn{sc4}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p4.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	// Turn last connection to connecting.
	bg.handleSubConnStateChange(sc4, connectivity.Connecting)
	p5 := <-cc.newPickerCh
	for i := 0; i < 5; i++ {
		if _, _, err := p5.Pick(context.Background(), balancer.PickOptions{}); err != balancer.ErrNoSubConnAvailable {
			t.Fatalf("want pick error %v, got %v", balancer.ErrNoSubConnAvailable, err)
		}
	}

	// Turn all connections down.
	bg.handleSubConnStateChange(sc4, connectivity.TransientFailure)
	p6 := <-cc.newPickerCh
	for i := 0; i < 5; i++ {
		if _, _, err := p6.Pick(context.Background(), balancer.PickOptions{}); err != balancer.ErrTransientFailure {
			t.Fatalf("want pick error %v, got %v", balancer.ErrTransientFailure, err)
		}
	}
}

// 2 balancers with different weights.
func TestBalancerGroup_TwoRR_DifferentWeight_MoreBackends(t *testing.T) {
	cc := newTestClientConn(t)
	bg := newBalancerGroup(cc, nil)

	// Add two balancers to group and send two resolved addresses to both
	// balancers.
	bg.add(testBalancerIDs[0], 2, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:2])
	sc1 := <-cc.newSubConnCh
	sc2 := <-cc.newSubConnCh

	bg.add(testBalancerIDs[1], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[1], testBackendAddrs[2:4])
	sc3 := <-cc.newSubConnCh
	sc4 := <-cc.newSubConnCh

	// Send state changes for both subconns.
	bg.handleSubConnStateChange(sc1, connectivity.Connecting)
	bg.handleSubConnStateChange(sc1, connectivity.Ready)
	bg.handleSubConnStateChange(sc2, connectivity.Connecting)
	bg.handleSubConnStateChange(sc2, connectivity.Ready)
	bg.handleSubConnStateChange(sc3, connectivity.Connecting)
	bg.handleSubConnStateChange(sc3, connectivity.Ready)
	bg.handleSubConnStateChange(sc4, connectivity.Connecting)
	bg.handleSubConnStateChange(sc4, connectivity.Ready)

	// Test roundrobin on the last picker.
	p1 := <-cc.newPickerCh
	want := []balancer.SubConn{sc1, sc1, sc2, sc2, sc3, sc4}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p1.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}
}

// totally 3 balancers, add/remove balancer.
func TestBalancerGroup_ThreeRR_RemoveBalancer(t *testing.T) {
	cc := newTestClientConn(t)
	bg := newBalancerGroup(cc, nil)

	// Add three balancers to group and send one resolved address to both
	// balancers.
	bg.add(testBalancerIDs[0], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:1])
	sc1 := <-cc.newSubConnCh

	bg.add(testBalancerIDs[1], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[1], testBackendAddrs[1:2])
	sc2 := <-cc.newSubConnCh

	bg.add(testBalancerIDs[2], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[2], testBackendAddrs[1:2])
	sc3 := <-cc.newSubConnCh

	// Send state changes for both subconns.
	bg.handleSubConnStateChange(sc1, connectivity.Connecting)
	bg.handleSubConnStateChange(sc1, connectivity.Ready)
	bg.handleSubConnStateChange(sc2, connectivity.Connecting)
	bg.handleSubConnStateChange(sc2, connectivity.Ready)
	bg.handleSubConnStateChange(sc3, connectivity.Connecting)
	bg.handleSubConnStateChange(sc3, connectivity.Ready)

	p1 := <-cc.newPickerCh
	want := []balancer.SubConn{sc1, sc2, sc3}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p1.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	// Remove the second balancer, while the others two are ready.
	bg.remove(testBalancerIDs[1])
	scToRemove := <-cc.removeSubConnCh
	if !reflect.DeepEqual(scToRemove, sc2) {
		t.Fatalf("RemoveSubConn, want %v, got %v", sc2, scToRemove)
	}
	p2 := <-cc.newPickerCh
	want = []balancer.SubConn{sc1, sc3}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p2.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	// move balancer 3 into transient failure.
	bg.handleSubConnStateChange(sc3, connectivity.TransientFailure)
	// Remove the first balancer, while the third is transient failure.
	bg.remove(testBalancerIDs[0])
	scToRemove = <-cc.removeSubConnCh
	if !reflect.DeepEqual(scToRemove, sc1) {
		t.Fatalf("RemoveSubConn, want %v, got %v", sc1, scToRemove)
	}
	p3 := <-cc.newPickerCh
	for i := 0; i < 5; i++ {
		if _, _, err := p3.Pick(context.Background(), balancer.PickOptions{}); err != balancer.ErrTransientFailure {
			t.Fatalf("want pick error %v, got %v", balancer.ErrTransientFailure, err)
		}
	}
}

// 2 balancers, change balancer weight.
func TestBalancerGroup_TwoRR_ChangeWeight_MoreBackends(t *testing.T) {
	cc := newTestClientConn(t)
	bg := newBalancerGroup(cc, nil)

	// Add two balancers to group and send two resolved addresses to both
	// balancers.
	bg.add(testBalancerIDs[0], 2, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:2])
	sc1 := <-cc.newSubConnCh
	sc2 := <-cc.newSubConnCh

	bg.add(testBalancerIDs[1], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[1], testBackendAddrs[2:4])
	sc3 := <-cc.newSubConnCh
	sc4 := <-cc.newSubConnCh

	// Send state changes for both subconns.
	bg.handleSubConnStateChange(sc1, connectivity.Connecting)
	bg.handleSubConnStateChange(sc1, connectivity.Ready)
	bg.handleSubConnStateChange(sc2, connectivity.Connecting)
	bg.handleSubConnStateChange(sc2, connectivity.Ready)
	bg.handleSubConnStateChange(sc3, connectivity.Connecting)
	bg.handleSubConnStateChange(sc3, connectivity.Ready)
	bg.handleSubConnStateChange(sc4, connectivity.Connecting)
	bg.handleSubConnStateChange(sc4, connectivity.Ready)

	// Test roundrobin on the last picker.
	p1 := <-cc.newPickerCh
	want := []balancer.SubConn{sc1, sc1, sc2, sc2, sc3, sc4}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p1.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}

	bg.changeWeight(testBalancerIDs[0], 3)

	// Test roundrobin with new weight.
	p2 := <-cc.newPickerCh
	want = []balancer.SubConn{sc1, sc1, sc1, sc2, sc2, sc2, sc3, sc4}
	if err := isRoundRobin(want, func() balancer.SubConn {
		sc, _, _ := p2.Pick(context.Background(), balancer.PickOptions{})
		return sc
	}); err != nil {
		t.Fatalf("want %v, got %v", want, err)
	}
}

func TestBalancerGroup_LoadReport(t *testing.T) {
	testLoadStore := newTestLoadStore()

	cc := newTestClientConn(t)
	bg := newBalancerGroup(cc, testLoadStore)

	backendToBalancerID := make(map[balancer.SubConn]internal.Locality)

	// Add two balancers to group and send two resolved addresses to both
	// balancers.
	bg.add(testBalancerIDs[0], 2, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[0], testBackendAddrs[0:2])
	sc1 := <-cc.newSubConnCh
	sc2 := <-cc.newSubConnCh
	backendToBalancerID[sc1] = testBalancerIDs[0]
	backendToBalancerID[sc2] = testBalancerIDs[0]

	bg.add(testBalancerIDs[1], 1, rrBuilder)
	bg.handleResolvedAddrs(testBalancerIDs[1], testBackendAddrs[2:4])
	sc3 := <-cc.newSubConnCh
	sc4 := <-cc.newSubConnCh
	backendToBalancerID[sc3] = testBalancerIDs[1]
	backendToBalancerID[sc4] = testBalancerIDs[1]

	// Send state changes for both subconns.
	bg.handleSubConnStateChange(sc1, connectivity.Connecting)
	bg.handleSubConnStateChange(sc1, connectivity.Ready)
	bg.handleSubConnStateChange(sc2, connectivity.Connecting)
	bg.handleSubConnStateChange(sc2, connectivity.Ready)
	bg.handleSubConnStateChange(sc3, connectivity.Connecting)
	bg.handleSubConnStateChange(sc3, connectivity.Ready)
	bg.handleSubConnStateChange(sc4, connectivity.Connecting)
	bg.handleSubConnStateChange(sc4, connectivity.Ready)

	// Test roundrobin on the last picker.
	p1 := <-cc.newPickerCh
	var (
		wantStart []internal.Locality
		wantEnd   []internal.Locality
		wantCost  []testServerLoad
	)
	for i := 0; i < 10; i++ {
		sc, done, _ := p1.Pick(context.Background(), balancer.PickOptions{})
		locality := backendToBalancerID[sc]
		wantStart = append(wantStart, locality)
		if done != nil && sc != sc1 {
			done(balancer.DoneInfo{
				ServerLoad: &orcapb.OrcaLoadReport{
					CpuUtilization:           10,
					MemUtilization:           5,
					RequestCostOrUtilization: map[string]float64{"pi": 3.14},
				},
			})
			wantEnd = append(wantEnd, locality)
			wantCost = append(wantCost,
				testServerLoad{name: serverLoadCPUName, d: 10},
				testServerLoad{name: serverLoadMemoryName, d: 5},
				testServerLoad{name: "pi", d: 3.14})
		}
	}

	if !reflect.DeepEqual(testLoadStore.callsStarted, wantStart) {
		t.Fatalf("want started: %v, got: %v", testLoadStore.callsStarted, wantStart)
	}
	if !reflect.DeepEqual(testLoadStore.callsEnded, wantEnd) {
		t.Fatalf("want ended: %v, got: %v", testLoadStore.callsEnded, wantEnd)
	}
	if !reflect.DeepEqual(testLoadStore.callsCost, wantCost) {
		t.Fatalf("want cost: %v, got: %v", testLoadStore.callsCost, wantCost)
	}
}
