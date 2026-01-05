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

package remote

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testNetwork               = "tcp"
	testAddrWithoutPort       = "this-is-my-addr.without-port"
	testAddrWithPort          = "this-is-my-addr.without-port:123"
	testPort                  = "123"
	ip1                       = "1.2.3.4"
	ip2                       = "5.6.7.8"
	ip3                       = "9.0.1.2"
	randSeed            int64 = 123456789
)

var (
	errMockLookupHost        = errors.New("this is a mocked error")
	testLookupResult         = []string{ip1, ip2, ip3}
	testLookupResultWithPort = []string{net.JoinHostPort(ip1, testPort), net.JoinHostPort(ip2, testPort), net.JoinHostPort(ip3, testPort)}
)

type mockDialContext struct {
	mock.Mock

	addrFrequencyMu sync.Mutex
	addrFrequency   map[string]int
}

func newMockDialContext(acceptableAddresses []string) *mockDialContext {
	m := &mockDialContext{
		addrFrequencyMu: sync.Mutex{},
		addrFrequency:   make(map[string]int),
	}
	for _, acceptableAddr := range acceptableAddresses {
		m.On("dialContext", mock.Anything, mock.Anything, acceptableAddr).Return(nil, nil)
	}
	return m
}

func (dc *mockDialContext) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dc.addrFrequencyMu.Lock()
	defer dc.addrFrequencyMu.Unlock()
	args := dc.MethodCalled("dialContext", ctx, network, addr)
	dc.addrFrequency[addr]++
	return nil, args.Error(1)
}

func (dc *mockDialContext) getCount(addr string) int {
	dc.addrFrequencyMu.Lock()
	defer dc.addrFrequencyMu.Unlock()
	return dc.addrFrequency[addr]
}

type mockedLookupHost struct {
	withErr bool
	result  []string
}

func (lh *mockedLookupHost) LookupHost(context.Context, string) ([]string, error) {
	if lh.withErr {
		return nil, errMockLookupHost
	}
	return lh.result, nil
}

func createDialContextWithRoundRobinDNS(dialContext config.DialContextFunc, resolver hostResolver, r *rand.Rand) dialContextWithRoundRobinDNS {
	return dialContextWithRoundRobinDNS{
		dialContext: dialContext,
		resolver:    resolver,
		rand:        r,
	}
}

func TestDialContextWithRandomConnections(t *testing.T) {
	numberOfRuns := 2 * len(testLookupResult)
	var mdc *mockDialContext
	testCases := map[string]struct {
		addr  string
		setup func() dialContextWithRoundRobinDNS
		check func()
	}{
		"if address contains no port call default DealContext": {
			addr: testAddrWithoutPort,
			setup: func() dialContextWithRoundRobinDNS {
				mdc = newMockDialContext([]string{testAddrWithoutPort})
				return createDialContextWithRoundRobinDNS(mdc.dialContext, &mockedLookupHost{withErr: false}, rand.New(rand.NewSource(time.Now().Unix())))
			},
			check: func() {
				require.Equal(t, numberOfRuns, mdc.getCount(testAddrWithoutPort))
			},
		},
		"if lookup host returns error call default DealContext": {
			addr: testAddrWithPort,
			setup: func() dialContextWithRoundRobinDNS {
				mdc = newMockDialContext([]string{testAddrWithPort})
				return createDialContextWithRoundRobinDNS(mdc.dialContext, &mockedLookupHost{withErr: true}, rand.New(rand.NewSource(time.Now().Unix())))
			},
			check: func() {
				require.Equal(t, numberOfRuns, mdc.getCount(testAddrWithPort))
			},
		},
		"if lookup returns no addresses call default DealContext": {
			addr: testAddrWithPort,
			setup: func() dialContextWithRoundRobinDNS {
				mdc = newMockDialContext([]string{testAddrWithPort})
				return createDialContextWithRoundRobinDNS(mdc.dialContext, &mockedLookupHost{}, rand.New(rand.NewSource(time.Now().Unix())))
			},
			check: func() {
				require.Equal(t, numberOfRuns, mdc.getCount(testAddrWithPort))
			},
		},
		"if lookup host is successful, shuffle results": {
			addr: testAddrWithPort,
			setup: func() dialContextWithRoundRobinDNS {
				mdc = newMockDialContext(testLookupResultWithPort)
				return createDialContextWithRoundRobinDNS(mdc.dialContext, &mockedLookupHost{result: testLookupResult}, rand.New(rand.NewSource(randSeed)))
			},
			check: func() {
				// we ensure that not all runs will choose the first element of the lookup
				require.NotEqual(t, numberOfRuns, mdc.getCount(testLookupResultWithPort[0]))
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			dc := tc.setup()
			require.NotNil(t, dc)
			for range numberOfRuns {
				_, err := dc.dialContextFn()(context.Background(), testNetwork, tc.addr)
				require.NoError(t, err)
			}
			tc.check()
		})
	}
}
