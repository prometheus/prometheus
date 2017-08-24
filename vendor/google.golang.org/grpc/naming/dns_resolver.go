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

package naming

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/grpclog"
)

const (
	defaultPort = "443"
	defaultFreq = time.Minute * 30
)

var (
	errMissingAddr  = errors.New("missing address")
	errWatcherClose = errors.New("watcher has been closed")
)

// NewDNSResolverWithFreq creates a DNS Resolver that can resolve DNS names, and
// create watchers that poll the DNS server using the frequency set by freq.
func NewDNSResolverWithFreq(freq time.Duration) (Resolver, error) {
	return &dnsResolver{freq: freq}, nil
}

// NewDNSResolver creates a DNS Resolver that can resolve DNS names, and create
// watchers that poll the DNS server using the default frequency defined by defaultFreq.
func NewDNSResolver() (Resolver, error) {
	return NewDNSResolverWithFreq(defaultFreq)
}

// dnsResolver handles name resolution for names following the DNS scheme
type dnsResolver struct {
	// frequency of polling the DNS server that the watchers created by this resolver will use.
	freq time.Duration
}

// formatIP returns ok = false if addr is not a valid textual representation of an IP address.
// If addr is an IPv4 address, return the addr and ok = true.
// If addr is an IPv6 address, return the addr enclosed in square brackets and ok = true.
func formatIP(addr string) (addrIP string, ok bool) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return "", false
	}
	if ip.To4() != nil {
		return addr, true
	}
	return "[" + addr + "]", true
}

// parseTarget takes the user input target string, returns formatted host and port info.
// If target doesn't specify a port, set the port to be the defaultPort.
// If target is in IPv6 format and host-name is enclosed in sqarue brackets, brackets
// are strippd when setting the host.
// examples:
// target: "www.google.com" returns host: "www.google.com", port: "443"
// target: "ipv4-host:80" returns host: "ipv4-host", port: "80"
// target: "[ipv6-host]" returns host: "ipv6-host", port: "443"
// target: ":80" returns host: "localhost", port: "80"
// target: ":" returns host: "localhost", port: "443"
func parseTarget(target string) (host, port string, err error) {
	if target == "" {
		return "", "", errMissingAddr
	}

	if ip := net.ParseIP(target); ip != nil {
		// target is an IPv4 or IPv6(without brackets) address
		return target, defaultPort, nil
	}
	if host, port, err := net.SplitHostPort(target); err == nil {
		// target has port, i.e ipv4-host:port, [ipv6-host]:port, host-name:port
		if host == "" {
			// Keep consistent with net.Dial(): If the host is empty, as in ":80", the local system is assumed.
			host = "localhost"
		}
		if port == "" {
			// If the port field is empty(target ends with colon), e.g. "[::1]:", defaultPort is used.
			port = defaultPort
		}
		return host, port, nil
	}
	if host, port, err := net.SplitHostPort(target + ":" + defaultPort); err == nil {
		// target doesn't have port
		return host, port, nil
	}
	return "", "", fmt.Errorf("invalid target address %v", target)
}

// Resolve creates a watcher that watches the name resolution of the target.
func (r *dnsResolver) Resolve(target string) (Watcher, error) {
	host, port, err := parseTarget(target)
	if err != nil {
		return nil, err
	}

	if net.ParseIP(host) != nil {
		ipWatcher := &ipWatcher{
			updateChan: make(chan *Update, 1),
		}
		host, _ = formatIP(host)
		ipWatcher.updateChan <- &Update{Op: Add, Addr: host + ":" + port}
		return ipWatcher, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &dnsWatcher{
		r:      r,
		host:   host,
		port:   port,
		ctx:    ctx,
		cancel: cancel,
		t:      time.NewTimer(0),
	}, nil
}

// dnsWatcher watches for the name resolution update for a specific target
type dnsWatcher struct {
	r    *dnsResolver
	host string
	port string
	// The latest resolved address list
	curAddrs []*Update
	ctx      context.Context
	cancel   context.CancelFunc
	t        *time.Timer
}

// ipWatcher watches for the name resolution update for an IP address.
type ipWatcher struct {
	updateChan chan *Update
}

// Next returns the adrress resolution Update for the target. For IP address,
// the resolution is itself, thus polling name server is unncessary. Therefore,
// Next() will return an Update the first time it is called, and will be blocked
// for all following calls as no Update exisits until watcher is closed.
func (i *ipWatcher) Next() ([]*Update, error) {
	u, ok := <-i.updateChan
	if !ok {
		return nil, errWatcherClose
	}
	return []*Update{u}, nil
}

// Close closes the ipWatcher.
func (i *ipWatcher) Close() {
	close(i.updateChan)
}

// AddressType indicates the address type returned by name resolution.
type AddressType uint8

const (
	// Backend indicates the server is a backend server.
	Backend AddressType = iota
	// GRPCLB indicates the server is a grpclb load balancer.
	GRPCLB
)

// AddrMetadataGRPCLB contains the information the name resolver for grpclb should provide. The
// name resolver used by the grpclb balancer is required to provide this type of metadata in
// its address updates.
type AddrMetadataGRPCLB struct {
	// AddrType is the type of server (grpc load balancer or backend).
	AddrType AddressType
	// ServerName is the name of the grpc load balancer. Used for authentication.
	ServerName string
}

// compileUpdate compares the old resolved addresses and newly resolved addresses,
// and generates an update list
func (w *dnsWatcher) compileUpdate(newAddrs []*Update) []*Update {
	update := make(map[Update]bool)
	for _, u := range newAddrs {
		update[*u] = true
	}
	for _, u := range w.curAddrs {
		if _, ok := update[*u]; ok {
			delete(update, *u)
			continue
		}
		update[Update{Addr: u.Addr, Op: Delete, Metadata: u.Metadata}] = true
	}
	res := make([]*Update, 0, len(update))
	for k := range update {
		tmp := k
		res = append(res, &tmp)
	}
	return res
}

func (w *dnsWatcher) lookupSRV() []*Update {
	var newAddrs []*Update
	_, srvs, err := lookupSRV(w.ctx, "grpclb", "tcp", w.host)
	if err != nil {
		grpclog.Infof("grpc: failed dns SRV record lookup due to %v.\n", err)
		return nil
	}
	for _, s := range srvs {
		lbAddrs, err := lookupHost(w.ctx, s.Target)
		if err != nil {
			grpclog.Warningf("grpc: failed load banlacer address dns lookup due to %v.\n", err)
			continue
		}
		for _, a := range lbAddrs {
			a, ok := formatIP(a)
			if !ok {
				grpclog.Errorf("grpc: failed IP parsing due to %v.\n", err)
				continue
			}
			newAddrs = append(newAddrs, &Update{Addr: a + ":" + strconv.Itoa(int(s.Port)),
				Metadata: AddrMetadataGRPCLB{AddrType: GRPCLB, ServerName: s.Target}})
		}
	}
	return newAddrs
}

func (w *dnsWatcher) lookupHost() []*Update {
	var newAddrs []*Update
	addrs, err := lookupHost(w.ctx, w.host)
	if err != nil {
		grpclog.Warningf("grpc: failed dns A record lookup due to %v.\n", err)
		return nil
	}
	for _, a := range addrs {
		a, ok := formatIP(a)
		if !ok {
			grpclog.Errorf("grpc: failed IP parsing due to %v.\n", err)
			continue
		}
		newAddrs = append(newAddrs, &Update{Addr: a + ":" + w.port})
	}
	return newAddrs
}

func (w *dnsWatcher) lookup() []*Update {
	newAddrs := w.lookupSRV()
	if newAddrs == nil {
		// If failed to get any balancer address (either no corresponding SRV for the
		// target, or caused by failure during resolution/parsing of the balancer target),
		// return any A record info available.
		newAddrs = w.lookupHost()
	}
	result := w.compileUpdate(newAddrs)
	w.curAddrs = newAddrs
	return result
}

// Next returns the resolved address update(delta) for the target. If there's no
// change, it will sleep for 30 mins and try to resolve again after that.
func (w *dnsWatcher) Next() ([]*Update, error) {
	for {
		select {
		case <-w.ctx.Done():
			return nil, errWatcherClose
		case <-w.t.C:
		}
		result := w.lookup()
		// Next lookup should happen after an interval defined by w.r.freq.
		w.t.Reset(w.r.freq)
		if len(result) > 0 {
			return result, nil
		}
	}
}

func (w *dnsWatcher) Close() {
	w.cancel()
}
