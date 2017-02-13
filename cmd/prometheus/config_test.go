// Copyright 2015 The Prometheus Authors
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

package main

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		input []string
		valid bool
	}{
		{
			input: []string{},
			valid: true,
		},
		{
			input: []string{"-web.external-url", ""},
			valid: true,
		},
		{
			input: []string{"-web.external-url", "http://proxy.com/prometheus"},
			valid: true,
		},
		{
			input: []string{"-web.external-url", "'https://url/prometheus'"},
			valid: false,
		},
		{
			input: []string{"-alertmanager.url", ""},
			valid: true,
		},
		{
			input: []string{"-alertmanager.url", "http://alertmanager.company.com"},
			valid: true,
		},
		{
			input: []string{"-alertmanager.url", "alertmanager.company.com"},
			valid: false,
		},
		{
			input: []string{"-alertmanager.url", "https://double--dash.de"},
			valid: true,
		},
	}

	for i, test := range tests {
		// reset "immutable" config
		cfg.prometheusURL = ""
		cfg.alertmanagerURLs = stringset{}

		err := parse(test.input)
		if test.valid && err != nil {
			t.Errorf("%d. expected input to be valid, got %s", i, err)
		} else if !test.valid && err == nil {
			t.Errorf("%d. expected input to be invalid", i)
		}
	}
}

func TestParseAlertmanagerURLToConfig(t *testing.T) {
	tests := []struct {
		url      string
		username string
		password string
	}{
		{
			url:      "http://alertmanager.company.com",
			username: "",
			password: "",
		},
		{
			url:      "https://user:password@alertmanager.company.com",
			username: "user",
			password: "password",
		},
	}

	for i, test := range tests {
		acfg, err := parseAlertmanagerURLToConfig(test.url)
		if err != nil {
			t.Errorf("%d. expected alertmanager URL to be valid, got %s", i, err)
		}

		if acfg.HTTPClientConfig.BasicAuth != nil {
			if test.username != acfg.HTTPClientConfig.BasicAuth.Username {
				t.Errorf("%d. expected alertmanagerConfig username to be %q, got %q",
					i, test.username, acfg.HTTPClientConfig.BasicAuth.Username)
			}

			if test.password != acfg.HTTPClientConfig.BasicAuth.Password {
				t.Errorf("%d. expected alertmanagerConfig password to be %q, got %q", i,
					test.password, acfg.HTTPClientConfig.BasicAuth.Username)
			}
			continue
		}

		if test.username != "" || test.password != "" {
			t.Errorf("%d. expected alertmanagerConfig to have basicAuth filled, but was not", i)
		}
	}
}
