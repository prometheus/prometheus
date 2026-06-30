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
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func newTestExpander(typ string, lookup ipLookupFunc) *addressExpander {
	return newAddressExpander("test", &ResolveAddressesConfig{
		Enabled:         true,
		RefreshInterval: model.Duration(time.Second),
		Type:            typ,
	}, nil, nil, make(chan struct{}, 1), lookup)
}

func staticLookup(addrs ...string) ipLookupFunc {
	return func(_ context.Context, _ string) ([]netip.Addr, error) {
		out := make([]netip.Addr, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, netip.MustParseAddr(a))
		}
		return out, nil
	}
}

func TestExpandTargetLiteralIPPassthrough(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, staticLookup("192.0.2.1"))
	for _, addr := range []string{"192.0.2.10:9090", "[2001:db8::1]:9090", "127.0.0.1"} {
		t.Run(addr, func(t *testing.T) {
			in := model.LabelSet{model.AddressLabel: model.LabelValue(addr)}
			got := e.expandTarget(in)
			require.Equal(t, []model.LabelSet{in}, got)
		})
	}
}

func TestExpandTargetUnresolvedKeepsFQDN(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, staticLookup("192.0.2.1"))
	in := model.LabelSet{model.AddressLabel: "host.example.com:9090"}
	got := e.expandTarget(in)
	// No cache entry yet: the FQDN target is kept untouched.
	require.Equal(t, []model.LabelSet{in}, got)
}

func TestExpandTargetSingleAddress(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, nil)
	e.seen["host.example.com"] = struct{}{}
	e.merge(map[string][]netip.Addr{"host.example.com": {netip.MustParseAddr("192.0.2.1")}})

	in := model.LabelSet{
		model.AddressLabel: "host.example.com:9090",
		"__meta_extra":     "kept",
	}
	got := e.expandTarget(in)
	require.Equal(t, []model.LabelSet{{
		model.AddressLabel:        "192.0.2.1:9090",
		"__meta_extra":            "kept",
		resolveAddressFQDNLabel:   "host.example.com",
		resolveAddressIPLabel:     "192.0.2.1",
		resolveAddressFamilyLabel: "4",
	}}, got)
}

func TestExpandTargetMultipleAddressesAuto(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, nil)
	e.merge(map[string][]netip.Addr{"host.example.com": {
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("2001:db8::1"),
	}})

	in := model.LabelSet{model.AddressLabel: "host.example.com:9090"}
	got := e.expandTarget(in)
	require.Equal(t, []model.LabelSet{
		{
			model.AddressLabel:        "192.0.2.1:9090",
			resolveAddressFQDNLabel:   "host.example.com",
			resolveAddressIPLabel:     "192.0.2.1",
			resolveAddressFamilyLabel: "4",
		},
		{
			model.AddressLabel:        "[2001:db8::1]:9090",
			resolveAddressFQDNLabel:   "host.example.com",
			resolveAddressIPLabel:     "2001:db8::1",
			resolveAddressFamilyLabel: "6",
		},
	}, got)
}

func TestExpandTargetNoPort(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, nil)
	e.merge(map[string][]netip.Addr{"host.example.com": {netip.MustParseAddr("192.0.2.1")}})

	got := e.expandTarget(model.LabelSet{model.AddressLabel: "host.example.com"})
	require.Len(t, got, 1)
	require.Equal(t, model.LabelValue("192.0.2.1"), got[0][model.AddressLabel])
}

func TestResolveHostTypeFiltering(t *testing.T) {
	lookup := staticLookup("192.0.2.1", "2001:db8::1", "192.0.2.2")
	for _, tc := range []struct {
		typ      string
		expected []netip.Addr
	}{
		{resolveTypeA, []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")}},
		{resolveTypeAAAA, []netip.Addr{netip.MustParseAddr("2001:db8::1")}},
		{resolveTypeAuto, []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2"), netip.MustParseAddr("2001:db8::1")}},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			e := newTestExpander(tc.typ, lookup)
			got, err := e.resolveHost(context.Background(), "host.example.com")
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestResolveAllKeepLastGood(t *testing.T) {
	failing := false
	e := newTestExpander(resolveTypeAuto, func(_ context.Context, _ string) ([]netip.Addr, error) {
		if failing {
			return nil, errors.New("boom")
		}
		return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil
	})
	e.seen["host.example.com"] = struct{}{}

	e.resolveAll(context.Background())
	got, ok := e.lookupCached("host.example.com")
	require.True(t, ok)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, got)

	// A failed refresh must not overwrite the previous good answer.
	failing = true
	e.resolveAll(context.Background())
	got, ok = e.lookupCached("host.example.com")
	require.True(t, ok)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, got)
}

func TestResolveAllTriggersSendOnChange(t *testing.T) {
	trigger := make(chan struct{}, 1)
	e := newAddressExpander("test", &ResolveAddressesConfig{
		Enabled: true, RefreshInterval: model.Duration(time.Second), Type: resolveTypeAuto,
	}, nil, nil, trigger, staticLookup("192.0.2.1"))
	e.seen["host.example.com"] = struct{}{}

	e.resolveAll(context.Background())
	select {
	case <-trigger:
	default:
		t.Fatal("expected triggerSend after cache change")
	}

	// A second resolution with the same answer must not trigger a resend.
	e.resolveAll(context.Background())
	select {
	case <-trigger:
		t.Fatal("unexpected triggerSend without cache change")
	default:
	}
}

func TestExpandGroupPreservesLabelsAndSource(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, nil)
	e.merge(map[string][]netip.Addr{"host.example.com": {
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
	}})

	tg := &targetgroup.Group{
		Source:  "src",
		Labels:  model.LabelSet{"job": "demo"},
		Targets: []model.LabelSet{{model.AddressLabel: "host.example.com:9090"}},
	}
	got := e.expandGroup(tg)
	require.Equal(t, "src", got.Source)
	require.Equal(t, model.LabelSet{"job": "demo"}, got.Labels)
	require.Len(t, got.Targets, 2)
	// Input group is not mutated.
	require.Len(t, tg.Targets, 1)
}

func TestObserveNudges(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, nil)
	e.observe("host.example.com")
	select {
	case <-e.nudge:
	default:
		t.Fatal("expected nudge on first observation")
	}
	// A repeated observation does not nudge again.
	e.observe("host.example.com")
	select {
	case <-e.nudge:
		t.Fatal("unexpected nudge for already-seen host")
	default:
	}
}

// TestEndCyclePrunesDisappearedHost proves H1: a host that disappears from the
// target set is pruned from seen and cache and is no longer resolved.
func TestEndCyclePrunesDisappearedHost(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, staticLookup("192.0.2.1"))

	// First cycle: two hosts observed and resolved.
	e.observe("a.example.com")
	e.observe("b.example.com")
	e.merge(map[string][]netip.Addr{
		"a.example.com": {netip.MustParseAddr("192.0.2.1")},
		"b.example.com": {netip.MustParseAddr("192.0.2.2")},
	})
	e.endCycle()

	_, okA := e.lookupCached("a.example.com")
	_, okB := e.lookupCached("b.example.com")
	require.True(t, okA)
	require.True(t, okB)

	// Second cycle: only a.example.com is observed; b disappears.
	e.observe("a.example.com")
	e.endCycle()

	e.mu.RLock()
	_, seenA := e.seen["a.example.com"]
	_, seenB := e.seen["b.example.com"]
	e.mu.RUnlock()
	require.True(t, seenA, "a.example.com must remain in seen")
	require.False(t, seenB, "b.example.com must be pruned from seen")

	_, cacheA := e.lookupCached("a.example.com")
	_, cacheB := e.lookupCached("b.example.com")
	require.True(t, cacheA, "a.example.com must remain cached")
	require.False(t, cacheB, "b.example.com must be pruned from cache")

	// A pruned host is no longer resolved: resolveAll only touches seen hosts.
	resolved := map[string]struct{}{}
	var mu sync.Mutex
	e.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		mu.Lock()
		resolved[host] = struct{}{}
		mu.Unlock()
		return []netip.Addr{netip.MustParseAddr("192.0.2.9")}, nil
	}
	e.resolveAll(context.Background())
	mu.Lock()
	_, resolvedB := resolved["b.example.com"]
	mu.Unlock()
	require.False(t, resolvedB, "pruned host must not be resolved again")
}

// TestExpandGroupUnchangedReturnsSamePointer proves M2: when nothing expands,
// expandGroup returns the original group pointer (no allocation).
func TestExpandGroupUnchangedReturnsSamePointer(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, nil)

	tg := &targetgroup.Group{
		Source: "src",
		Targets: []model.LabelSet{
			{model.AddressLabel: "192.0.2.1:9090"},        // Literal IP.
			{model.AddressLabel: "host.example.com:9090"}, // Unresolved FQDN.
		},
	}
	got := e.expandGroup(tg)
	require.Same(t, tg, got, "unchanged group must be returned by pointer")

	// observe is still called for the FQDN so resolution is triggered.
	e.mu.RLock()
	_, seen := e.seen["host.example.com"]
	e.mu.RUnlock()
	require.True(t, seen, "FQDN must be observed even when nothing expands")
}

// TestResolveHostContextCancellationNoFailure proves M4: a lookup error caused
// by context cancellation does not increment ResolveLookupFailures.
func TestResolveHostContextCancellationNoFailure(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewManagerMetrics(reg, "test")
	require.NoError(t, err)

	e := newAddressExpander("set", &ResolveAddressesConfig{
		Enabled: true, RefreshInterval: model.Duration(time.Second), Type: resolveTypeAuto,
	}, metrics, nil, make(chan struct{}, 1), func(ctx context.Context, _ string) ([]netip.Addr, error) {
		return nil, ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = e.resolveHost(ctx, "host.example.com")
	require.Error(t, err)

	require.Equal(t, 0.0, client_testutil.ToFloat64(metrics.ResolveLookupFailures.WithLabelValues("set")))
}

// TestResolveHostContextCancellationMidLookup checks that an error returned
// while the context is cancelled (not pre-cancelled) is also not counted.
func TestResolveHostContextCancellationMidLookup(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewManagerMetrics(reg, "test")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	e := newAddressExpander("set", &ResolveAddressesConfig{
		Enabled: true, RefreshInterval: model.Duration(time.Second), Type: resolveTypeAuto,
	}, metrics, nil, make(chan struct{}, 1), func(_ context.Context, _ string) ([]netip.Addr, error) {
		cancel()
		return nil, errors.New("lookup aborted")
	})

	_, err = e.resolveHost(ctx, "host.example.com")
	require.Error(t, err)
	require.Equal(t, 0.0, client_testutil.ToFloat64(metrics.ResolveLookupFailures.WithLabelValues("set")))
}

// TestResolveHostGenuineFailureCounts is the counterpart: a real failure with a
// live context does increment the failure counter.
func TestResolveHostGenuineFailureCounts(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewManagerMetrics(reg, "test")
	require.NoError(t, err)

	e := newAddressExpander("set", &ResolveAddressesConfig{
		Enabled: true, RefreshInterval: model.Duration(time.Second), Type: resolveTypeAuto,
	}, metrics, nil, make(chan struct{}, 1), func(_ context.Context, _ string) ([]netip.Addr, error) {
		return nil, errors.New("nxdomain")
	})

	_, err = e.resolveHost(context.Background(), "host.example.com")
	require.Error(t, err)
	require.Equal(t, 1.0, client_testutil.ToFloat64(metrics.ResolveLookupFailures.WithLabelValues("set")))
}

// TestResolveHostNotFoundDoesNotCount proves that a definitive NXDOMAIN answer
// is not counted as a lookup failure, since the host simply reverts to its FQDN
// form rather than indicating a resolution problem.
func TestResolveHostNotFoundDoesNotCount(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewManagerMetrics(reg, "test")
	require.NoError(t, err)

	notFound := &net.DNSError{Err: "no such host", Name: "host.example.com", IsNotFound: true}
	e := newAddressExpander("set", &ResolveAddressesConfig{
		Enabled: true, RefreshInterval: model.Duration(time.Second), Type: resolveTypeAuto,
	}, metrics, nil, make(chan struct{}, 1), func(_ context.Context, _ string) ([]netip.Addr, error) {
		return nil, notFound
	})

	_, err = e.resolveHost(context.Background(), "host.example.com")
	require.Error(t, err)
	require.Equal(t, 0.0, client_testutil.ToFloat64(metrics.ResolveLookupFailures.WithLabelValues("set")))
}

// TestExpandTargetBracketedIPv6NoPort proves L1: a bracketed IPv6 literal
// without a port is treated as a literal IP and passes through unchanged.
func TestExpandTargetBracketedIPv6NoPort(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, staticLookup("192.0.2.1"))
	in := model.LabelSet{model.AddressLabel: "[2001:db8::1]"}
	got := e.expandTarget(in)
	require.Equal(t, []model.LabelSet{in}, got)

	// It must not have been observed as an FQDN.
	e.mu.RLock()
	_, seen := e.seen["[2001:db8::1]"]
	e.mu.RUnlock()
	require.False(t, seen, "bracketed IPv6 literal must not be observed as FQDN")
}

// TestExpandTargetIPv6NoPort proves L6: an FQDN with no port resolving to an
// IPv6 address yields a bare (bracketless) address.
func TestExpandTargetIPv6NoPort(t *testing.T) {
	e := newTestExpander(resolveTypeAuto, nil)
	e.merge(map[string][]netip.Addr{"host.example.com": {netip.MustParseAddr("2001:db8::1")}})

	got := e.expandTarget(model.LabelSet{model.AddressLabel: "host.example.com"})
	require.Len(t, got, 1)
	require.Equal(t, model.LabelValue("2001:db8::1"), got[0][model.AddressLabel])
	require.Equal(t, model.LabelValue("6"), got[0][resolveAddressFamilyLabel])
}

// TestMaxResolvedAddressesTruncation proves H2: a resolved answer larger than
// the cap is truncated deterministically and the truncation metric increments.
func TestMaxResolvedAddressesTruncation(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewManagerMetrics(reg, "test")
	require.NoError(t, err)

	e := newAddressExpander("set", &ResolveAddressesConfig{
		Enabled: true, RefreshInterval: model.Duration(time.Second), Type: resolveTypeAuto,
		MaxResolvedAddresses: 2,
	}, metrics, nil, make(chan struct{}, 1), staticLookup("192.0.2.3", "192.0.2.1", "192.0.2.2"))

	got, err := e.resolveHost(context.Background(), "host.example.com")
	require.NoError(t, err)
	// Deterministic: sorted then truncated to the first two addresses.
	require.Equal(t, []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
	}, got)
	require.Equal(t, 1.0, client_testutil.ToFloat64(metrics.ResolveAddressTruncations.WithLabelValues("set")))

	// A second over-cap resolution increments the metric again but warns once.
	_, err = e.resolveHost(context.Background(), "host.example.com")
	require.NoError(t, err)
	require.Equal(t, 2.0, client_testutil.ToFloat64(metrics.ResolveAddressTruncations.WithLabelValues("set")))
}

// TestResolveAllEvictsOnDefinitiveNegative proves that a definitive negative
// answer (an empty result or an NXDOMAIN error) for a still-present host evicts
// the last good IPs so the target reverts to its FQDN form, while a transient
// error keeps the last good IPs.
func TestResolveAllEvictsOnDefinitiveNegative(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", Name: "host.example.com", IsNotFound: true}
	for _, tc := range []struct {
		name    string
		fail    func() ([]netip.Addr, error)
		evicted bool
	}{
		{"empty answer", func() ([]netip.Addr, error) { return nil, nil }, true},
		{"nxdomain", func() ([]netip.Addr, error) { return nil, notFound }, true},
		{"transient error", func() ([]netip.Addr, error) { return nil, errors.New("boom") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fail := false
			e := newTestExpander(resolveTypeAuto, func(_ context.Context, _ string) ([]netip.Addr, error) {
				if fail {
					return tc.fail()
				}
				return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil
			})
			e.seen["host.example.com"] = struct{}{}

			e.resolveAll(context.Background())
			got, ok := e.lookupCached("host.example.com")
			require.True(t, ok)
			require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, got)

			fail = true
			e.resolveAll(context.Background())
			_, ok = e.lookupCached("host.example.com")
			if tc.evicted {
				require.False(t, ok, "definitive negative must evict the cached answer")
			} else {
				require.True(t, ok, "transient failure must keep the last good answer")
			}
		})
	}
}

// TestNewAddressExpanderZeroIntervalClamp proves L9: a non-positive interval is
// clamped to the default so a direct construction cannot panic on
// time.NewTicker(0).
func TestNewAddressExpanderZeroIntervalClamp(t *testing.T) {
	e := newAddressExpander("set", &ResolveAddressesConfig{
		Enabled: true, RefreshInterval: 0, Type: resolveTypeAuto,
	}, nil, nil, make(chan struct{}, 1), staticLookup("192.0.2.1"))
	require.Equal(t, time.Duration(DefaultResolveAddressesConfig.RefreshInterval), e.interval)

	// The default-clamped interval must be usable by a ticker without panicking.
	require.NotPanics(t, func() {
		ticker := time.NewTicker(e.interval)
		ticker.Stop()
	})
}

// TestSnapshotSeedRoundTrip proves M1's building blocks: snapshot copies state
// and seed installs it into a fresh expander.
func TestSnapshotSeedRoundTrip(t *testing.T) {
	src := newTestExpander(resolveTypeAuto, nil)
	src.observe("host.example.com")
	src.merge(map[string][]netip.Addr{"host.example.com": {netip.MustParseAddr("192.0.2.1")}})

	cache, seen := src.snapshot()

	dst := newTestExpander(resolveTypeAuto, nil)
	dst.seed(cache, seen)

	got, ok := dst.lookupCached("host.example.com")
	require.True(t, ok)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, got)

	// The snapshot is a copy: mutating the source cache does not affect the seed.
	src.merge(map[string][]netip.Addr{"host.example.com": {netip.MustParseAddr("203.0.113.9")}})
	got, _ = dst.lookupCached("host.example.com")
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, got)
}
