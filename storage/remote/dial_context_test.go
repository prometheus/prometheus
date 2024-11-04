package remote

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testNetwork         = "tcp"
	testAddrWithoutPort = "this-is-my-addr.without-port"
	testAddrWithPort    = "this-is-my-addr.without-port:123"
	testPort            = "123"
	ip1                 = "1.2.3.4"
	ip2                 = "5.6.7.8"
	ip3                 = "9.0.1.2"
)

var (
	errMockLookupHost        = fmt.Errorf("this is a mocked error")
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

func (lh *mockedLookupHost) lookupHost(context.Context, string) ([]string, error) {
	if lh.withErr {
		return nil, errMockLookupHost
	}
	return lh.result, nil
}

func TestDialContextWithRandomConnections(t *testing.T) {
	numberOfRuns := 2 * len(testLookupResult)
	var (
		mdc *mockDialContext
	)
	testCases := map[string]struct {
		addr                              string
		setup                             func() customDialContext
		check                             func()
		withLookupHostErr                 bool
		expectedDefaultContextArguments   []string
		unexpectedDefaultContextArguments []string
	}{
		"if address contains no port call default DealContext": {
			addr: testAddrWithoutPort,
			setup: func() customDialContext {
				dc := newDialContextWithRandomConnections()
				mdc = newMockDialContext([]string{testAddrWithoutPort})
				dc.setDefaultDialContext(mdc.dialContext)
				return dc
			},
			check: func() {
				require.Equal(t, numberOfRuns, mdc.getCount(testAddrWithoutPort))
			},
		},
		"if lookup host returns error call default DealContext": {
			addr: testAddrWithPort,
			setup: func() customDialContext {
				dc := newDialContextWithRandomConnections()
				mdc = newMockDialContext([]string{testAddrWithPort})
				dc.setDefaultDialContext(mdc.dialContext)
				dc.setResolver(&mockedLookupHost{withErr: true})
				return dc
			},
			check: func() {
				require.Equal(t, numberOfRuns, mdc.getCount(testAddrWithPort))
			},
		},
		"if lookup host is successful, shuffle results": {
			addr: testAddrWithPort,
			setup: func() customDialContext {
				dc := newDialContextWithRandomConnections()
				mdc = newMockDialContext(testLookupResultWithPort)
				dc.setDefaultDialContext(mdc.dialContext)
				dc.setResolver(&mockedLookupHost{result: testLookupResult})
				return dc
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
			for i := 0; i < numberOfRuns; i++ {
				_, err := dc.dialContextFn()(context.Background(), testNetwork, tc.addr)
				require.NoError(t, err)
			}
			tc.check()
		})
	}
}
