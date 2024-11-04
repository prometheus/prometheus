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
	lookupHost(context.Context, string) ([]string, error)
}

type defaultHostResolver struct{}

func (hr *defaultHostResolver) lookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
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
		resolver:    &defaultHostResolver{},
	}
}

func (dc *dialContextWithRandomConnections) setDefaultDialContext(defaultDialContext config.DialContextFunc) {
	dc.dialContext = defaultDialContext
}

func (dc *dialContextWithRandomConnections) setResolver(resolver hostResolver) {
	dc.resolver = resolver
}

func (dc *dialContextWithRandomConnections) dialContextFn() config.DialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return dc.dialContext(ctx, network, addr)
		}

		addrs, err := dc.resolver.lookupHost(ctx, host)
		if err != nil {
			return dc.dialContext(ctx, network, addr)
		}

		randomAddr := addrs[rand.Intn(len(addrs))]
		randomAddr = net.JoinHostPort(randomAddr, port)
		return dc.dialContext(ctx, network, randomAddr)
	}
}
