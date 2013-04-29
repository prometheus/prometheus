package retrieval

import (
	"net"
	"net/http"
	"time"
)

// NewDeadlineClient returns a new http.Client which will time out long running
// requests.
func NewDeadlineClient(timeout time.Duration) http.Client {
	return http.Client{
		Transport: &http.Transport{
			// We need to disable keepalive, becasue we set a deadline on the
			// underlying connection.
			DisableKeepAlives: true,
			Dial: func(netw, addr string) (c net.Conn, err error) {
				start := time.Now()

				c, err = net.DialTimeout(netw, addr, timeout)

				if err == nil {
					c.SetDeadline(start.Add(timeout))
				}

				return
			},
		},
	}
}
