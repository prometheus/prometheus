// Copyright 2013 The Prometheus Authors
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

package httputil

import (
	"net"
	"net/http"
	"time"
)

// NewDeadlineClient returns a new http.Client which will time out long running
// requests.
func NewDeadlineClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			// We need to disable keepalive, because we set a deadline on the
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
