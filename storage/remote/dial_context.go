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
