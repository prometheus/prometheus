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
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/common/config"
)

type hostResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

type dialContextWithRoundRobinDNS struct {
	dialContext config.DialContextFunc
	resolver    hostResolver
	rand        *rand.Rand
}

// newDialContextWithRoundRobinDNS creates a new dialContextWithRoundRobinDNS.
// We discourage creating new instances of struct dialContextWithRoundRobinDNS by explicitly setting its members,
// except for testing purposes, and recommend using newDialContextWithRoundRobinDNS.
func newDialContextWithRoundRobinDNS() *dialContextWithRoundRobinDNS {
	return &dialContextWithRoundRobinDNS{
		dialContext: http.DefaultTransport.(*http.Transport).DialContext,
		resolver:    net.DefaultResolver,
		rand:        rand.New(rand.NewSource(time.Now().Unix())),
	}
}

func (dc *dialContextWithRoundRobinDNS) dialContextFn() config.DialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return dc.dialContext(ctx, network, addr)
		}

		addrs, err := dc.resolver.LookupHost(ctx, host)
		if err != nil || len(addrs) == 0 {
			return dc.dialContext(ctx, network, addr)
		}

		randomAddr := net.JoinHostPort(addrs[dc.rand.Intn(len(addrs))], port)
		return dc.dialContext(ctx, network, randomAddr)
	}
}
