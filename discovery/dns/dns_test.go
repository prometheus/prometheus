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

	"github.com/go-kit/log"
	"github.com/miekg/dns"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

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
				{
					Source: "web.example.com.",
					Targets: []model.LabelSet{
						{
							"__address__":                  "192.0.2.2:80",
							"__meta_dns_name":              "web.example.com.",
							"__meta_dns_srv_record_target": "",
							"__meta_dns_srv_record_port":   "",
							"__meta_dns_mx_record_target":  "",
						},
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
				{
					Source: "web.example.com.",
					Targets: []model.LabelSet{
						{
							"__address__":                  "[::1]:80",
							"__meta_dns_name":              "web.example.com.",
							"__meta_dns_srv_record_target": "",
							"__meta_dns_srv_record_port":   "",
							"__meta_dns_mx_record_target":  "",
						},
					},
				},
			},
		},
		{
			name: "SRV record query",
			config: SDConfig{
				Names:           []string{"_mysql._tcp.db.example.com."},
				Type:            "SRV",
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
				{
					Source: "_mysql._tcp.db.example.com.",
					Targets: []model.LabelSet{
						{
							"__address__":                  "db1.example.com:3306",
							"__meta_dns_name":              "_mysql._tcp.db.example.com.",
							"__meta_dns_srv_record_target": "db1.example.com.",
							"__meta_dns_srv_record_port":   "3306",
							"__meta_dns_mx_record_target":  "",
						},
						{
							"__address__":                  "db2.example.com:3306",
							"__meta_dns_name":              "_mysql._tcp.db.example.com.",
							"__meta_dns_srv_record_target": "db2.example.com.",
							"__meta_dns_srv_record_port":   "3306",
							"__meta_dns_mx_record_target":  "",
						},
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
				{
					Source: "_mysql._tcp.db.example.com.",
					Targets: []model.LabelSet{
						{
							"__address__":                  "db1.example.com:3306",
							"__meta_dns_name":              "_mysql._tcp.db.example.com.",
							"__meta_dns_srv_record_target": "db1.example.com.",
							"__meta_dns_srv_record_port":   "3306",
							"__meta_dns_mx_record_target":  "",
						},
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
				{
					Source: "_mysql._tcp.db.example.com.",
				},
			},
		},
		{
			name: "MX record query",
			config: SDConfig{
				Names:           []string{"example.com."},
				Type:            "MX",
				Port:            25,
				RefreshInterval: model.Duration(time.Minute),
			},
			lookup: func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
				return &dns.Msg{
						Answer: []dns.RR{
							&dns.MX{Preference: 0, Mx: "smtp1.example.com."},
							&dns.MX{Preference: 10, Mx: "smtp2.example.com."},
						},
					},
					nil
			},
			expected: []*targetgroup.Group{
				{
					Source: "example.com.",
					Targets: []model.LabelSet{
						{
							"__address__":                  "smtp1.example.com:25",
							"__meta_dns_name":              "example.com.",
							"__meta_dns_srv_record_target": "",
							"__meta_dns_srv_record_port":   "",
							"__meta_dns_mx_record_target":  "smtp1.example.com.",
						},
						{
							"__address__":                  "smtp2.example.com:25",
							"__meta_dns_name":              "example.com.",
							"__meta_dns_srv_record_target": "",
							"__meta_dns_srv_record_port":   "",
							"__meta_dns_mx_record_target":  "smtp2.example.com.",
						},
					},
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

func TestSDConfigUnmarshalYAML(t *testing.T) {
	marshal := func(c SDConfig) []byte {
		d, err := yaml.Marshal(c)
		if err != nil {
			panic(err)
		}
		return d
	}

	unmarshal := func(d []byte) func(interface{}) error {
		return func(o interface{}) error {
			return yaml.Unmarshal(d, o)
		}
	}

	cases := []struct {
		name      string
		input     SDConfig
		expectErr bool
	}{
		{
			name: "valid srv",
			input: SDConfig{
				Names: []string{"a.example.com", "b.example.com"},
				Type:  "SRV",
			},
			expectErr: false,
		},
		{
			name: "valid a",
			input: SDConfig{
				Names: []string{"a.example.com", "b.example.com"},
				Type:  "A",
				Port:  5300,
			},
			expectErr: false,
		},
		{
			name: "valid aaaa",
			input: SDConfig{
				Names: []string{"a.example.com", "b.example.com"},
				Type:  "AAAA",
				Port:  5300,
			},
			expectErr: false,
		},
		{
			name: "invalid a without port",
			input: SDConfig{
				Names: []string{"a.example.com", "b.example.com"},
				Type:  "A",
			},
			expectErr: true,
		},
		{
			name: "invalid aaaa without port",
			input: SDConfig{
				Names: []string{"a.example.com", "b.example.com"},
				Type:  "AAAA",
			},
			expectErr: true,
		},
		{
			name: "invalid empty names",
			input: SDConfig{
				Names: []string{},
				Type:  "AAAA",
			},
			expectErr: true,
		},
		{
			name: "invalid unknown dns type",
			input: SDConfig{
				Names: []string{"a.example.com", "b.example.com"},
				Type:  "PTR",
			},
			expectErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var config SDConfig
			d := marshal(c.input)
			err := config.UnmarshalYAML(unmarshal(d))
			require.Equal(t, c.expectErr, err != nil)
		})
	}
}
