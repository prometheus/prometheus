/*
 *
 * Copyright 2018 gRPC authors.
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

// Package dns implements a dns resolver to be installed as the default resolver
// in grpc.
package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/internal/backoff"
	"google.golang.org/grpc/internal/grpcrand"
	"google.golang.org/grpc/resolver"
)

func init() {
	resolver.Register(NewBuilder())
}

const (
	defaultPort       = "443"
	defaultFreq       = time.Minute * 30
	defaultDNSSvrPort = "53"
	golang            = "GO"
	// In DNS, service config is encoded in a TXT record via the mechanism
	// described in RFC-1464 using the attribute name grpc_config.
	txtAttribute = "grpc_config="
)

var (
	errMissingAddr = errors.New("dns resolver: missing address")

	// Addresses ending with a colon that is supposed to be the separator
	// between host and port is not allowed.  E.g. "::" is a valid address as
	// it is an IPv6 address (host only) and "[::]:" is invalid as it ends with
	// a colon as the host and port separator
	errEndsWithColon = errors.New("dns resolver: missing port after port-separator colon")
)

var (
	defaultResolver netResolver = net.DefaultResolver
)

var customAuthorityDialler = func(authority string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, authority)
	}
}

var customAuthorityResolver = func(authority string) (netResolver, error) {
	host, port, err := parseTarget(authority, defaultDNSSvrPort)
	if err != nil {
		return nil, err
	}

	authorityWithPort := net.JoinHostPort(host, port)

	return &net.Resolver{
		PreferGo: true,
		Dial:     customAuthorityDialler(authorityWithPort),
	}, nil
}

// NewBuilder creates a dnsBuilder which is used to factory DNS resolvers.
func NewBuilder() resolver.Builder {
	return &dnsBuilder{minFreq: defaultFreq}
}

type dnsBuilder struct {
	// minimum frequency of polling the DNS server.
	minFreq time.Duration
}

// Build creates and starts a DNS resolver that watches the name resolution of the target.
func (b *dnsBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOption) (resolver.Resolver, error) {
	host, port, err := parseTarget(target.Endpoint, defaultPort)
	if err != nil {
		return nil, err
	}

	// IP address.
	if net.ParseIP(host) != nil {
		host, _ = formatIP(host)
		addr := []resolver.Address{{Addr: host + ":" + port}}
		i := &ipResolver{
			cc: cc,
			ip: addr,
			rn: make(chan struct{}, 1),
			q:  make(chan struct{}),
		}
		cc.NewAddress(addr)
		go i.watcher()
		return i, nil
	}

	// DNS address (non-IP).
	ctx, cancel := context.WithCancel(context.Background())
	d := &dnsResolver{
		freq:                 b.minFreq,
		backoff:              backoff.Exponential{MaxDelay: b.minFreq},
		host:                 host,
		port:                 port,
		ctx:                  ctx,
		cancel:               cancel,
		cc:                   cc,
		t:                    time.NewTimer(0),
		rn:                   make(chan struct{}, 1),
		disableServiceConfig: opts.DisableServiceConfig,
	}

	if target.Authority == "" {
		d.resolver = defaultResolver
	} else {
		d.resolver, err = customAuthorityResolver(target.Authority)
		if err != nil {
			return nil, err
		}
	}

	d.wg.Add(1)
	go d.watcher()
	return d, nil
}

// Scheme returns the naming scheme of this resolver builder, which is "dns".
func (b *dnsBuilder) Scheme() string {
	return "dns"
}

type netResolver interface {
	LookupHost(ctx context.Context, host string) (addrs []string, err error)
	LookupSRV(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error)
	LookupTXT(ctx context.Context, name string) (txts []string, err error)
}

// ipResolver watches for the name resolution update for an IP address.
type ipResolver struct {
	cc resolver.ClientConn
	ip []resolver.Address
	// rn channel is used by ResolveNow() to force an immediate resolution of the target.
	rn chan struct{}
	q  chan struct{}
}

// ResolveNow resend the address it stores, no resolution is needed.
func (i *ipResolver) ResolveNow(opt resolver.ResolveNowOption) {
	select {
	case i.rn <- struct{}{}:
	default:
	}
}

// Close closes the ipResolver.
func (i *ipResolver) Close() {
	close(i.q)
}

func (i *ipResolver) watcher() {
	for {
		select {
		case <-i.rn:
			i.cc.NewAddress(i.ip)
		case <-i.q:
			return
		}
	}
}

// dnsResolver watches for the name resolution update for a non-IP target.
type dnsResolver struct {
	freq       time.Duration
	backoff    backoff.Exponential
	retryCount int
	host       string
	port       string
	resolver   netResolver
	ctx        context.Context
	cancel     context.CancelFunc
	cc         resolver.ClientConn
	// rn channel is used by ResolveNow() to force an immediate resolution of the target.
	rn chan struct{}
	t  *time.Timer
	// wg is used to enforce Close() to return after the watcher() goroutine has finished.
	// Otherwise, data race will be possible. [Race Example] in dns_resolver_test we
	// replace the real lookup functions with mocked ones to facilitate testing.
	// If Close() doesn't wait for watcher() goroutine finishes, race detector sometimes
	// will warns lookup (READ the lookup function pointers) inside watcher() goroutine
	// has data race with replaceNetFunc (WRITE the lookup function pointers).
	wg                   sync.WaitGroup
	disableServiceConfig bool
}

// ResolveNow invoke an immediate resolution of the target that this dnsResolver watches.
func (d *dnsResolver) ResolveNow(opt resolver.ResolveNowOption) {
	select {
	case d.rn <- struct{}{}:
	default:
	}
}

// Close closes the dnsResolver.
func (d *dnsResolver) Close() {
	d.cancel()
	d.wg.Wait()
	d.t.Stop()
}

func (d *dnsResolver) watcher() {
	defer d.wg.Done()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-d.t.C:
		case <-d.rn:
		}
		result, sc := d.lookup()
		// Next lookup should happen within an interval defined by d.freq. It may be
		// more often due to exponential retry on empty address list.
		if len(result) == 0 {
			d.retryCount++
			d.t.Reset(d.backoff.Backoff(d.retryCount))
		} else {
			d.retryCount = 0
			d.t.Reset(d.freq)
		}
		d.cc.NewServiceConfig(sc)
		d.cc.NewAddress(result)
	}
}

func (d *dnsResolver) lookupSRV() []resolver.Address {
	var newAddrs []resolver.Address
	_, srvs, err := d.resolver.LookupSRV(d.ctx, "grpclb", "tcp", d.host)
	if err != nil {
		grpclog.Infof("grpc: failed dns SRV record lookup due to %v.\n", err)
		return nil
	}
	for _, s := range srvs {
		lbAddrs, err := d.resolver.LookupHost(d.ctx, s.Target)
		if err != nil {
			grpclog.Infof("grpc: failed load balancer address dns lookup due to %v.\n", err)
			continue
		}
		for _, a := range lbAddrs {
			a, ok := formatIP(a)
			if !ok {
				grpclog.Errorf("grpc: failed IP parsing due to %v.\n", err)
				continue
			}
			addr := a + ":" + strconv.Itoa(int(s.Port))
			newAddrs = append(newAddrs, resolver.Address{Addr: addr, Type: resolver.GRPCLB, ServerName: s.Target})
		}
	}
	return newAddrs
}

func (d *dnsResolver) lookupTXT() string {
	ss, err := d.resolver.LookupTXT(d.ctx, d.host)
	if err != nil {
		grpclog.Infof("grpc: failed dns TXT record lookup due to %v.\n", err)
		return ""
	}
	var res string
	for _, s := range ss {
		res += s
	}

	// TXT record must have "grpc_config=" attribute in order to be used as service config.
	if !strings.HasPrefix(res, txtAttribute) {
		grpclog.Warningf("grpc: TXT record %v missing %v attribute", res, txtAttribute)
		return ""
	}
	return strings.TrimPrefix(res, txtAttribute)
}

func (d *dnsResolver) lookupHost() []resolver.Address {
	var newAddrs []resolver.Address
	addrs, err := d.resolver.LookupHost(d.ctx, d.host)
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
		addr := a + ":" + d.port
		newAddrs = append(newAddrs, resolver.Address{Addr: addr})
	}
	return newAddrs
}

func (d *dnsResolver) lookup() ([]resolver.Address, string) {
	newAddrs := d.lookupSRV()
	// Support fallback to non-balancer address.
	newAddrs = append(newAddrs, d.lookupHost()...)
	if d.disableServiceConfig {
		return newAddrs, ""
	}
	sc := d.lookupTXT()
	return newAddrs, canaryingSC(sc)
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

// parseTarget takes the user input target string and default port, returns formatted host and port info.
// If target doesn't specify a port, set the port to be the defaultPort.
// If target is in IPv6 format and host-name is enclosed in sqarue brackets, brackets
// are strippd when setting the host.
// examples:
// target: "www.google.com" defaultPort: "443" returns host: "www.google.com", port: "443"
// target: "ipv4-host:80" defaultPort: "443" returns host: "ipv4-host", port: "80"
// target: "[ipv6-host]" defaultPort: "443" returns host: "ipv6-host", port: "443"
// target: ":80" defaultPort: "443" returns host: "localhost", port: "80"
func parseTarget(target, defaultPort string) (host, port string, err error) {
	if target == "" {
		return "", "", errMissingAddr
	}
	if ip := net.ParseIP(target); ip != nil {
		// target is an IPv4 or IPv6(without brackets) address
		return target, defaultPort, nil
	}
	if host, port, err = net.SplitHostPort(target); err == nil {
		if port == "" {
			// If the port field is empty (target ends with colon), e.g. "[::1]:", this is an error.
			return "", "", errEndsWithColon
		}
		// target has port, i.e ipv4-host:port, [ipv6-host]:port, host-name:port
		if host == "" {
			// Keep consistent with net.Dial(): If the host is empty, as in ":80", the local system is assumed.
			host = "localhost"
		}
		return host, port, nil
	}
	if host, port, err = net.SplitHostPort(target + ":" + defaultPort); err == nil {
		// target doesn't have port
		return host, port, nil
	}
	return "", "", fmt.Errorf("invalid target address %v, error info: %v", target, err)
}

type rawChoice struct {
	ClientLanguage *[]string        `json:"clientLanguage,omitempty"`
	Percentage     *int             `json:"percentage,omitempty"`
	ClientHostName *[]string        `json:"clientHostName,omitempty"`
	ServiceConfig  *json.RawMessage `json:"serviceConfig,omitempty"`
}

func containsString(a *[]string, b string) bool {
	if a == nil {
		return true
	}
	for _, c := range *a {
		if c == b {
			return true
		}
	}
	return false
}

func chosenByPercentage(a *int) bool {
	if a == nil {
		return true
	}
	return grpcrand.Intn(100)+1 <= *a
}

func canaryingSC(js string) string {
	if js == "" {
		return ""
	}
	var rcs []rawChoice
	err := json.Unmarshal([]byte(js), &rcs)
	if err != nil {
		grpclog.Warningf("grpc: failed to parse service config json string due to %v.\n", err)
		return ""
	}
	cliHostname, err := os.Hostname()
	if err != nil {
		grpclog.Warningf("grpc: failed to get client hostname due to %v.\n", err)
		return ""
	}
	var sc string
	for _, c := range rcs {
		if !containsString(c.ClientLanguage, golang) ||
			!chosenByPercentage(c.Percentage) ||
			!containsString(c.ClientHostName, cliHostname) ||
			c.ServiceConfig == nil {
			continue
		}
		sc = string(*c.ServiceConfig)
		break
	}
	return sc
}
