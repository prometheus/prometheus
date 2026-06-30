// Copyright The Prometheus Authors
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
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Meta labels attached to targets that have been expanded from an FQDN.
const (
	resolveAddressFQDNLabel   = model.LabelName(model.MetaLabelPrefix + "resolve_address_fqdn")
	resolveAddressIPLabel     = model.LabelName(model.MetaLabelPrefix + "resolve_address_ip")
	resolveAddressFamilyLabel = model.LabelName(model.MetaLabelPrefix + "resolve_address_family")
)

// ipLookupFunc resolves a host to a set of IP addresses. It is a struct field
// so that tests can inject a deterministic resolver.
type ipLookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// defaultIPLookupFor returns a system-resolver lookup that queries only the IP
// family implied by the configured record type: "ip4" for A, "ip6" for AAAA
// and "ip" for auto. Restricting the network avoids querying the family that
// would be discarded anyway.
func defaultIPLookupFor(typ string) ipLookupFunc {
	network := "ip"
	switch strings.ToUpper(typ) {
	case resolveTypeA:
		network = "ip4"
	case resolveTypeAAAA:
		network = "ip6"
	}
	return func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, network, host)
	}
}

// isNotFound reports whether err is a definitive negative answer (NXDOMAIN or
// "no such host") rather than a transient resolver failure. Definitive
// negatives evict the cached answer so the target reverts to its FQDN form;
// transient failures keep the last good answer.
func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// addressExpander resolves the FQDNs observed in a target set into IP addresses
// and expands each FQDN target into one target per resolved IP. Resolution runs
// in a background goroutine and results are cached, so the synchronous expansion
// performed in Manager.allGroups() is a pure cache lookup.
type addressExpander struct {
	setName     string
	typ         string // Normalised to "A", "AAAA" or "AUTO".
	interval    time.Duration
	maxAddrs    int
	warnedTrunc map[string]struct{} // Hosts already warned about truncation.

	logger      *slog.Logger
	lookup      ipLookupFunc
	metrics     *Metrics
	triggerSend chan<- struct{}

	// nudge requests an out-of-band resolution when a new FQDN is observed.
	nudge chan struct{}

	mu    sync.RWMutex
	cache map[string][]netip.Addr // FQDN -> last good resolved IPs.
	seen  map[string]struct{}     // FQDNs observed in the current target state.
	// cycle accumulates the hosts observed during the in-progress allGroups
	// pass. endCycle promotes it to seen and prunes stale cache entries.
	cycle map[string]struct{}

	cancel context.CancelFunc
}

// newAddressExpander returns an expander for the given target set and config.
// A nil lookup falls back to the system resolver.
func newAddressExpander(setName string, cfg *ResolveAddressesConfig, metrics *Metrics, logger *slog.Logger, triggerSend chan<- struct{}, lookup ipLookupFunc) *addressExpander {
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	if lookup == nil {
		lookup = defaultIPLookupFor(cfg.Type)
	}
	interval := time.Duration(cfg.RefreshInterval)
	if interval <= 0 {
		// Guard against time.NewTicker(0) panics from a direct construction
		// that bypasses config validation.
		interval = time.Duration(DefaultResolveAddressesConfig.RefreshInterval)
	}
	maxAddrs := cfg.MaxResolvedAddresses
	if maxAddrs <= 0 {
		maxAddrs = defaultMaxResolvedAddresses
	}
	return &addressExpander{
		setName:     setName,
		typ:         strings.ToUpper(cfg.Type),
		interval:    interval,
		maxAddrs:    maxAddrs,
		warnedTrunc: map[string]struct{}{},
		logger:      logger,
		lookup:      lookup,
		metrics:     metrics,
		triggerSend: triggerSend,
		nudge:       make(chan struct{}, 1),
		cache:       map[string][]netip.Addr{},
		seen:        map[string]struct{}{},
		cycle:       map[string]struct{}{},
	}
}

// matchesConfig reports whether the expander already runs with the given config.
func (e *addressExpander) matchesConfig(cfg *ResolveAddressesConfig) bool {
	return e.typ == strings.ToUpper(cfg.Type) && e.interval == time.Duration(cfg.RefreshInterval)
}

// start launches the background resolution loop. It stops when ctx is cancelled
// or stop is called.
func (e *addressExpander) start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	go e.run(ctx)
}

// stop terminates the background resolution loop.
func (e *addressExpander) stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

func (e *addressExpander) run(ctx context.Context) {
	e.resolveAll(ctx)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.resolveAll(ctx)
		case <-e.nudge:
			e.resolveAll(ctx)
		}
	}
}

// resolveAll resolves every observed FQDN and, if any cached entry changed,
// signals the Manager to resend its targets.
func (e *addressExpander) resolveAll(ctx context.Context) {
	e.mu.RLock()
	hosts := make([]string, 0, len(e.seen))
	for h := range e.seen {
		hosts = append(hosts, h)
	}
	e.mu.RUnlock()

	if len(hosts) == 0 {
		return
	}

	var (
		wg      sync.WaitGroup
		resMu   sync.Mutex
		results = make(map[string][]netip.Addr, len(hosts))
		// sem bounds the per-refresh goroutine fan-out so a large target set
		// cannot spawn an unbounded number of concurrent lookups.
		sem = make(chan struct{}, min(8, len(hosts)))
	)
	wg.Add(len(hosts))
	for _, h := range hosts {
		go func(host string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			addrs, err := e.resolveHost(ctx, host)
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					return
				}
				if isNotFound(err) {
					// A definitive NXDOMAIN / "no such host" answer evicts the
					// host so its target reverts to the FQDN form rather than
					// scraping stale IPs.
					resMu.Lock()
					results[host] = nil
					resMu.Unlock()
					return
				}
				// Transient failure (timeout, SERVFAIL, ...): keep last good so
				// a blip does not flap existing targets.
				e.logger.Debug("Address resolution failed", "host", host, "err", err)
				return
			}
			// Record the answer, including an empty one: an empty answer evicts
			// the cached entry so the target reverts to its FQDN form.
			resMu.Lock()
			results[host] = addrs
			resMu.Unlock()
		}(h)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return
	}

	if e.merge(results) {
		select {
		case e.triggerSend <- struct{}{}:
		default:
		}
	}
}

// resolveHost resolves a single host and filters the answer by the configured
// record type.
func (e *addressExpander) resolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	// Skip the lookup entirely if the context is already cancelled.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.metrics != nil {
		e.metrics.ResolveLookups.WithLabelValues(e.setName).Inc()
	}
	addrs, err := e.lookup(ctx, host)
	if err != nil {
		// Do not count context cancellation (shutdown or restart) or a
		// definitive negative answer (NXDOMAIN / "no such host") as a lookup
		// failure: neither is a resolution problem. A not-found answer evicts
		// the host so its target reverts to the FQDN form.
		if e.metrics != nil && ctx.Err() == nil && !isNotFound(err) {
			e.metrics.ResolveLookupFailures.WithLabelValues(e.setName).Inc()
		}
		return nil, err
	}

	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		a = a.Unmap()
		if e.matchesType(a) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b netip.Addr) int { return a.Compare(b) })

	// Bound the fan-out from an untrusted DNS answer. Truncation is
	// deterministic because it happens after the sort.
	if len(out) > e.maxAddrs {
		e.warnTruncation(host, len(out))
		out = out[:e.maxAddrs]
	}
	return out, nil
}

// warnTruncation logs a single warning per host and increments the truncation
// metric whenever a host's resolved address count exceeds the configured cap.
func (e *addressExpander) warnTruncation(host string, count int) {
	if e.metrics != nil {
		e.metrics.ResolveAddressTruncations.WithLabelValues(e.setName).Inc()
	}
	e.mu.Lock()
	_, warned := e.warnedTrunc[host]
	if !warned {
		e.warnedTrunc[host] = struct{}{}
	}
	e.mu.Unlock()
	if !warned {
		e.logger.Warn("Truncating resolved addresses to max_resolved_addresses", "host", host, "count", count, "max", e.maxAddrs)
	}
}

// matchesType reports whether the address matches the configured record type.
// The default resolver is already network-aware (see defaultIPLookupFor), but
// this remains as defence for injected resolvers and the auto type.
func (e *addressExpander) matchesType(a netip.Addr) bool {
	switch e.typ {
	case resolveTypeA:
		return a.Is4()
	case resolveTypeAAAA:
		return a.Is6()
	default:
		return true
	}
}

// merge updates the cache with newly resolved addresses and reports whether any
// entry changed.
func (e *addressExpander) merge(results map[string][]netip.Addr) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	changed := false
	for host, addrs := range results {
		if len(addrs) == 0 {
			// A definitive empty answer evicts the cached entry so the target
			// falls back to its FQDN form.
			if _, ok := e.cache[host]; ok {
				delete(e.cache, host)
				changed = true
			}
			continue
		}
		if !slices.Equal(e.cache[host], addrs) {
			e.cache[host] = addrs
			changed = true
		}
	}
	return changed
}

// lookupCached returns the cached addresses for a host without performing I/O.
func (e *addressExpander) lookupCached(host string) ([]netip.Addr, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	addrs, ok := e.cache[host]
	return addrs, ok
}

// observe records a host as seen in the current cycle and nudges an out-of-band
// resolution the first time the host is encountered.
func (e *addressExpander) observe(host string) {
	e.mu.Lock()
	e.cycle[host] = struct{}{}
	_, ok := e.seen[host]
	if !ok {
		e.seen[host] = struct{}{}
	}
	e.mu.Unlock()
	if ok {
		return
	}

	select {
	case e.nudge <- struct{}{}:
	default:
	}
}

// endCycle finalises one allGroups pass with a mark-and-sweep: hosts observed
// during the pass become the new seen set, cache and truncation-warning entries
// for hosts that disappeared are pruned, and the cycle set is reset. It must be
// called exactly once per allGroups pass.
func (e *addressExpander) endCycle() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.seen = e.cycle
	for host := range e.cache {
		if _, ok := e.cycle[host]; !ok {
			delete(e.cache, host)
		}
	}
	for host := range e.warnedTrunc {
		if _, ok := e.cycle[host]; !ok {
			delete(e.warnedTrunc, host)
		}
	}
	e.cycle = map[string]struct{}{}
}

// snapshot returns copies of the current cache and seen sets. It is used to
// carry resolution state across an expander restart so a config-only change
// does not flap targets back to their FQDN form.
func (e *addressExpander) snapshot() (cache map[string][]netip.Addr, seen map[string]struct{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cache = make(map[string][]netip.Addr, len(e.cache))
	for h, addrs := range e.cache {
		cache[h] = slices.Clone(addrs)
	}
	seen = make(map[string]struct{}, len(e.seen))
	for h := range e.seen {
		seen[h] = struct{}{}
	}
	return cache, seen
}

// seed installs a previously captured cache and seen set, used to preserve
// resolution state across an expander restart. It must be called before start.
func (e *addressExpander) seed(cache map[string][]netip.Addr, seen map[string]struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cache != nil {
		e.cache = cache
	}
	if seen != nil {
		e.seen = seen
	}
}

// expandGroup returns a group with every FQDN target expanded into one target
// per resolved IP. Group-level Labels and Source are preserved. The input group
// is never mutated. When no target actually expands (all literal IPs, empty
// addresses, or not-yet-resolved FQDNs that pass through unchanged) the original
// tg pointer is returned to avoid an allocation on the hot path. observe is
// still called for FQDNs so resolution is triggered.
func (e *addressExpander) expandGroup(tg *targetgroup.Group) *targetgroup.Group {
	if tg == nil {
		return nil
	}

	// First pass: expand each target and detect whether anything changed.
	var (
		expanded = make([][]model.LabelSet, len(tg.Targets))
		changed  bool
	)
	for i, t := range tg.Targets {
		exp := e.expandTarget(t)
		expanded[i] = exp
		// A target is unchanged only when expansion yields exactly the same
		// single label set it started with.
		if len(exp) != 1 || !sameLabelSet(exp[0], t) {
			changed = true
		}
	}
	if !changed {
		return tg
	}

	out := &targetgroup.Group{
		Source:  tg.Source,
		Labels:  tg.Labels,
		Targets: make([]model.LabelSet, 0, len(tg.Targets)),
	}
	for _, exp := range expanded {
		out.Targets = append(out.Targets, exp...)
	}
	return out
}

// sameLabelSet reports whether two label sets have identical contents. It is
// used to detect when expandTarget returned its input unchanged so expandGroup
// can avoid allocating a new group.
func sameLabelSet(a, b model.LabelSet) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// expandTarget expands a single target. Literal IPs and not-yet-resolved FQDNs
// are returned unchanged (sharing the input map).
func (e *addressExpander) expandTarget(t model.LabelSet) []model.LabelSet {
	addrVal, ok := t[model.AddressLabel]
	if !ok || addrVal == "" {
		return []model.LabelSet{t}
	}

	addr := string(addrVal)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// No port present: treat the whole value as the host.
		host = addr
		port = ""
	}

	// Literal IP addresses pass through unchanged. Strip surrounding brackets
	// so a bracketed IPv6 literal without a port (e.g. "[2001:db8::1]") is
	// recognised as an IP rather than mistaken for an FQDN.
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return []model.LabelSet{t}
	}

	e.observe(host)

	ips, ok := e.lookupCached(host)
	if !ok || len(ips) == 0 {
		// Not resolved yet (or no good answer): keep the FQDN target so it
		// still scrapes by name until the first resolution lands.
		return []model.LabelSet{t}
	}

	expanded := make([]model.LabelSet, 0, len(ips))
	for _, ip := range ips {
		ls := t.Clone()
		ls[model.AddressLabel] = model.LabelValue(joinHostPort(ip.String(), port))
		ls[resolveAddressFQDNLabel] = model.LabelValue(host)
		ls[resolveAddressIPLabel] = model.LabelValue(ip.String())
		ls[resolveAddressFamilyLabel] = model.LabelValue(addressFamily(ip))
		expanded = append(expanded, ls)
	}
	return expanded
}

// joinHostPort joins host and port, keeping host alone when no port is set.
func joinHostPort(host, port string) string {
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

// addressFamily returns "4" for IPv4 addresses and "6" for IPv6 addresses.
func addressFamily(a netip.Addr) string {
	if a.Is4() {
		return "4"
	}
	return "6"
}
