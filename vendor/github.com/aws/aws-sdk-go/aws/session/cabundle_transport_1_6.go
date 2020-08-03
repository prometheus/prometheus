// +build !go1.7,go1.6

package session

import (
	"net"
	"net/http"
	"time"
)

// Transport that should be used when a custom CA bundle is specified with the
// SDK.
func getCABundleTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
