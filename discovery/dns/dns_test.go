// Copyright 2019 The Prometheus Authors
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

package dns

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/miekg/dns"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestDNS(t *testing.T) {
	testCases := []struct {
		name   string
		config SDConfig
		lookup func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error)

		expected []*targetgroup.Group
	}{
		{
			name: "A record query with error",
			config: SDConfig{
				Names:           []string{"web.example.com."},
				RefreshInterval: model.Duration(time.Minute),
				Port:            80,
				Type:            "A",
			},
			lookup: func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
				return nil, fmt.Errorf("some error")
			},
			expected: []*targetgroup.Group{},
		},
		{
			name: "A record query",
			config: SDConfig{
				Names:           []string{"web.example.com."},
				RefreshInterval: model.Duration(time.Minute),
				Port:            80,
				Type:            "A",
			},
			lookup: func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
				return &dns.Msg{
						Answer: []dns.RR{
							&dns.A{A: net.IPv4(192, 0, 2, 2)},
						},
					},
					nil
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "web.example.com.",
					Targets: []model.LabelSet{
						{"__address__": "192.0.2.2:80", "__meta_dns_name": "web.example.com."},
					},
				},
			},
		},
		{
			name: "AAAA record query",
			config: SDConfig{
				Names:           []string{"web.example.com."},
				RefreshInterval: model.Duration(time.Minute),
				Port:            80,
				Type:            "AAAA",
			},
			lookup: func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
				return &dns.Msg{
						Answer: []dns.RR{
							&dns.AAAA{AAAA: net.IPv6loopback},
						},
					},
					nil
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "web.example.com.",
					Targets: []model.LabelSet{
						{"__address__": "[::1]:80", "__meta_dns_name": "web.example.com."},
					},
				},
			},
		},
		{
			name: "SRV record query",
			config: SDConfig{
				Names:           []string{"_mysql._tcp.db.example.com."},
				RefreshInterval: model.Duration(time.Minute),
			},
			lookup: func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
				return &dns.Msg{
						Answer: []dns.RR{
							&dns.SRV{Port: 3306, Target: "db1.example.com."},
							&dns.SRV{Port: 3306, Target: "db2.example.com."},
						},
					},
					nil
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "_mysql._tcp.db.example.com.",
					Targets: []model.LabelSet{
						{"__address__": "db1.example.com:3306", "__meta_dns_name": "_mysql._tcp.db.example.com."},
						{"__address__": "db2.example.com:3306", "__meta_dns_name": "_mysql._tcp.db.example.com."},
					},
				},
			},
		},
		{
			name: "SRV record query with unsupported resource records",
			config: SDConfig{
				Names:           []string{"_mysql._tcp.db.example.com."},
				RefreshInterval: model.Duration(time.Minute),
			},
			lookup: func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
				return &dns.Msg{
						Answer: []dns.RR{
							&dns.SRV{Port: 3306, Target: "db1.example.com."},
							&dns.TXT{Txt: []string{"this should be discarded"}},
						},
					},
					nil
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "_mysql._tcp.db.example.com.",
					Targets: []model.LabelSet{
						{"__address__": "db1.example.com:3306", "__meta_dns_name": "_mysql._tcp.db.example.com."},
					},
				},
			},
		},
		{
			name: "SRV record query with empty answer (NXDOMAIN)",
			config: SDConfig{
				Names:           []string{"_mysql._tcp.db.example.com."},
				RefreshInterval: model.Duration(time.Minute),
			},
			lookup: func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
				return &dns.Msg{}, nil
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "_mysql._tcp.db.example.com.",
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sd := NewDiscovery(tc.config, nil)
			sd.lookupFn = tc.lookup

			tgs, err := sd.refresh(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expected, tgs)
		})
	}
}
