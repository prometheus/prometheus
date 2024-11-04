// Copyright 2016 The Prometheus Authors
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

	"github.com/prometheus/common/config"
)

type hostResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

type customDialContext interface {
	dialContextFn() config.DialContextFunc
}

type dialContextWithRandomConnections struct {
	dialContext config.DialContextFunc
	resolver    hostResolver
}

func newDialContextWithRandomConnections() *dialContextWithRandomConnections {
	return &dialContextWithRandomConnections{
		dialContext: http.DefaultTransport.(*http.Transport).DialContext,
		resolver:    net.DefaultResolver,
	}
}

func (dc *dialContextWithRandomConnections) dialContextFn() config.DialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return dc.dialContext(ctx, network, addr)
		}

		addrs, err := dc.resolver.LookupHost(ctx, host)
		if err != nil {
			return dc.dialContext(ctx, network, addr)
		}

		// We deliberately create a connection to a randomly selected IP returned by the lookup.
		// This way we prevent that in case of a certain number of concurrent remote-write requests
		// all requests are sent to the same IP.
		randomAddr := net.JoinHostPort(addrs[rand.Intn(len(addrs))], port)
		return dc.dialContext(ctx, network, randomAddr)
	}
}
