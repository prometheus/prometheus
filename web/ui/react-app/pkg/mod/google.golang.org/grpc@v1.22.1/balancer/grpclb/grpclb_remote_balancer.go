/*
 *
 * Copyright 2017 gRPC authors.
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
 *
 */

package grpclb

import (
	"context"
	"fmt"
	"io"
	"net"
	"reflect"
	"time"

	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	lbpb "google.golang.org/grpc/balancer/grpclb/grpc_lb_v1"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/internal"
	"google.golang.org/grpc/internal/channelz"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
)

// processServerList updates balaner's internal state, create/remove SubConns
// and regenerates picker using the received serverList.
func (lb *lbBalancer) processServerList(l *lbpb.ServerList) {
	if grpclog.V(2) {
		grpclog.Infof("lbBalancer: processing server list: %+v", l)
	}
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Set serverListReceived to true so fallback will not take effect if it has
	// not hit timeout.
	lb.serverListReceived = true

	// If the new server list == old server list, do nothing.
	if reflect.DeepEqual(lb.fullServerList, l.Servers) {
		if grpclog.V(2) {
			grpclog.Infof("lbBalancer: new serverlist same as the previous one, ignoring")
		}
		return
	}
	lb.fullServerList = l.Servers

	var backendAddrs []resolver.Address
	for i, s := range l.Servers {
		if s.Drop {
			continue
		}

		md := metadata.Pairs(lbTokeyKey, s.LoadBalanceToken)
		ip := net.IP(s.IpAddress)
		ipStr := ip.String()
		if ip.To4() == nil {
			// Add square brackets to ipv6 addresses, otherwise net.Dial() and
			// net.SplitHostPort() will return too many colons error.
			ipStr = fmt.Sprintf("[%s]", ipStr)
		}
		addr := resolver.Address{
			Addr:     fmt.Sprintf("%s:%d", ipStr, s.Port),
			Metadata: &md,
		}
		if grpclog.V(2) {
			grpclog.Infof("lbBalancer: server list entry[%d]: ipStr:|%s|, port:|%d|, load balancer token:|%v|",
				i, ipStr, s.Port, s.LoadBalanceToken)
		}
		backendAddrs = append(backendAddrs, addr)
	}

	// Call refreshSubConns to create/remove SubConns.  If we are in fallback,
	// this is also exiting fallback.
	lb.refreshSubConns(backendAddrs, false, lb.usePickFirst)
}

// refreshSubConns creates/removes SubConns with backendAddrs, and refreshes
// balancer state and picker.
//
// Caller must hold lb.mu.
func (lb *lbBalancer) refreshSubConns(backendAddrs []resolver.Address, fallback bool, pickFirst bool) {
	lb.inFallback = fallback

	opts := balancer.NewSubConnOptions{}
	if !fallback {
		opts.CredsBundle = lb.grpclbBackendCreds
	}

	lb.backendAddrs = backendAddrs
	lb.backendAddrsWithoutMetadata = nil

	if lb.usePickFirst != pickFirst {
		// Remove all SubConns when switching modes.
		for a, sc := range lb.subConns {
			if lb.usePickFirst {
				lb.cc.cc.RemoveSubConn(sc)
			} else {
				lb.cc.RemoveSubConn(sc)
			}
			delete(lb.subConns, a)
		}
		lb.usePickFirst = pickFirst
	}

	if lb.usePickFirst {
		var sc balancer.SubConn
		for _, sc = range lb.subConns {
			break
		}
		if sc != nil {
			sc.UpdateAddresses(backendAddrs)
			sc.Connect()
			return
		}
		// This bypasses the cc wrapper with SubConn cache.
		sc, err := lb.cc.cc.NewSubConn(backendAddrs, opts)
		if err != nil {
			grpclog.Warningf("grpclb: failed to create new SubConn: %v", err)
			return
		}
		sc.Connect()
		lb.subConns[backendAddrs[0]] = sc
		lb.scStates[sc] = connectivity.Idle
		return
	}

	// addrsSet is the set converted from backendAddrsWithoutMetadata, it's used to quick
	// lookup for an address.
	addrsSet := make(map[resolver.Address]struct{})
	// Create new SubConns.
	for _, addr := range backendAddrs {
		addrWithoutMD := addr
		addrWithoutMD.Metadata = nil
		addrsSet[addrWithoutMD] = struct{}{}
		lb.backendAddrsWithoutMetadata = append(lb.backendAddrsWithoutMetadata, addrWithoutMD)

		if _, ok := lb.subConns[addrWithoutMD]; !ok {
			// Use addrWithMD to create the SubConn.
			sc, err := lb.cc.NewSubConn([]resolver.Address{addr}, opts)
			if err != nil {
				grpclog.Warningf("grpclb: failed to create new SubConn: %v", err)
				continue
			}
			lb.subConns[addrWithoutMD] = sc // Use the addr without MD as key for the map.
			if _, ok := lb.scStates[sc]; !ok {
				// Only set state of new sc to IDLE. The state could already be
				// READY for cached SubConns.
				lb.scStates[sc] = connectivity.Idle
			}
			sc.Connect()
		}
	}

	for a, sc := range lb.subConns {
		// a was removed by resolver.
		if _, ok := addrsSet[a]; !ok {
			lb.cc.RemoveSubConn(sc)
			delete(lb.subConns, a)
			// Keep the state of this sc in b.scStates until sc's state becomes Shutdown.
			// The entry will be deleted in HandleSubConnStateChange.
		}
	}

	// Regenerate and update picker after refreshing subconns because with
	// cache, even if SubConn was newed/removed, there might be no state
	// changes (the subconn will be kept in cache, not actually
	// newed/removed).
	lb.updateStateAndPicker(true, true)
}

func (lb *lbBalancer) readServerList(s *balanceLoadClientStream) error {
	for {
		reply, err := s.Recv()
		if err != nil {
			if err == io.EOF {
				return errServerTerminatedConnection
			}
			return fmt.Errorf("grpclb: failed to recv server list: %v", err)
		}
		if serverList := reply.GetServerList(); serverList != nil {
			lb.processServerList(serverList)
		}
	}
}

func (lb *lbBalancer) sendLoadReport(s *balanceLoadClientStream, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-s.Context().Done():
			return
		}
		stats := lb.clientStats.toClientStats()
		t := time.Now()
		stats.Timestamp = &timestamppb.Timestamp{
			Seconds: t.Unix(),
			Nanos:   int32(t.Nanosecond()),
		}
		if err := s.Send(&lbpb.LoadBalanceRequest{
			LoadBalanceRequestType: &lbpb.LoadBalanceRequest_ClientStats{
				ClientStats: stats,
			},
		}); err != nil {
			return
		}
	}
}

func (lb *lbBalancer) callRemoteBalancer() (backoff bool, _ error) {
	lbClient := &loadBalancerClient{cc: lb.ccRemoteLB}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := lbClient.BalanceLoad(ctx, grpc.WaitForReady(true))
	if err != nil {
		return true, fmt.Errorf("grpclb: failed to perform RPC to the remote balancer %v", err)
	}
	lb.mu.Lock()
	lb.remoteBalancerConnected = true
	lb.mu.Unlock()

	// grpclb handshake on the stream.
	initReq := &lbpb.LoadBalanceRequest{
		LoadBalanceRequestType: &lbpb.LoadBalanceRequest_InitialRequest{
			InitialRequest: &lbpb.InitialLoadBalanceRequest{
				Name: lb.target,
			},
		},
	}
	if err := stream.Send(initReq); err != nil {
		return true, fmt.Errorf("grpclb: failed to send init request: %v", err)
	}
	reply, err := stream.Recv()
	if err != nil {
		return true, fmt.Errorf("grpclb: failed to recv init response: %v", err)
	}
	initResp := reply.GetInitialResponse()
	if initResp == nil {
		return true, fmt.Errorf("grpclb: reply from remote balancer did not include initial response")
	}
	if initResp.LoadBalancerDelegate != "" {
		return true, fmt.Errorf("grpclb: Delegation is not supported")
	}

	go func() {
		if d := convertDuration(initResp.ClientStatsReportInterval); d > 0 {
			lb.sendLoadReport(stream, d)
		}
	}()
	// No backoff if init req/resp handshake was successful.
	return false, lb.readServerList(stream)
}

func (lb *lbBalancer) watchRemoteBalancer() {
	var retryCount int
	for {
		doBackoff, err := lb.callRemoteBalancer()
		select {
		case <-lb.doneCh:
			return
		default:
			if err != nil {
				if err == errServerTerminatedConnection {
					grpclog.Info(err)
				} else {
					grpclog.Warning(err)
				}
			}
		}
		// Trigger a re-resolve when the stream errors.
		lb.cc.cc.ResolveNow(resolver.ResolveNowOption{})

		lb.mu.Lock()
		lb.remoteBalancerConnected = false
		lb.fullServerList = nil
		// Enter fallback when connection to remote balancer is lost, and the
		// aggregated state is not Ready.
		if !lb.inFallback && lb.state != connectivity.Ready {
			// Entering fallback.
			lb.refreshSubConns(lb.resolvedBackendAddrs, true, lb.usePickFirst)
		}
		lb.mu.Unlock()

		if !doBackoff {
			retryCount = 0
			continue
		}

		timer := time.NewTimer(lb.backoff.Backoff(retryCount))
		select {
		case <-timer.C:
		case <-lb.doneCh:
			timer.Stop()
			return
		}
		retryCount++
	}
}

func (lb *lbBalancer) dialRemoteLB(remoteLBName string) {
	var dopts []grpc.DialOption
	if creds := lb.opt.DialCreds; creds != nil {
		if err := creds.OverrideServerName(remoteLBName); err == nil {
			dopts = append(dopts, grpc.WithTransportCredentials(creds))
		} else {
			grpclog.Warningf("grpclb: failed to override the server name in the credentials: %v, using Insecure", err)
			dopts = append(dopts, grpc.WithInsecure())
		}
	} else if bundle := lb.grpclbClientConnCreds; bundle != nil {
		dopts = append(dopts, grpc.WithCredentialsBundle(bundle))
	} else {
		dopts = append(dopts, grpc.WithInsecure())
	}
	if lb.opt.Dialer != nil {
		dopts = append(dopts, grpc.WithContextDialer(lb.opt.Dialer))
	}
	// Explicitly set pickfirst as the balancer.
	dopts = append(dopts, grpc.WithBalancerName(grpc.PickFirstBalancerName))
	wrb := internal.WithResolverBuilder.(func(resolver.Builder) grpc.DialOption)
	dopts = append(dopts, wrb(lb.manualResolver))
	if channelz.IsOn() {
		dopts = append(dopts, grpc.WithChannelzParentID(lb.opt.ChannelzParentID))
	}

	// DialContext using manualResolver.Scheme, which is a random scheme
	// generated when init grpclb. The target scheme here is not important.
	//
	// The grpc dial target will be used by the creds (ALTS) as the authority,
	// so it has to be set to remoteLBName that comes from resolver.
	cc, err := grpc.DialContext(context.Background(), remoteLBName, dopts...)
	if err != nil {
		grpclog.Fatalf("failed to dial: %v", err)
	}
	lb.ccRemoteLB = cc
	go lb.watchRemoteBalancer()
}
