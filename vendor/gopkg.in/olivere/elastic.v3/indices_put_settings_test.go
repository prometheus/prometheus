// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestIndicesPutSettingsBuildURL(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tests := []struct {
		Indices  []string
		Expected string
	}{
		{
			[]string{},
			"/_settings",
		},
		{
			[]string{"*"},
			"/%2A/_settings",
		},
		{
			[]string{"store-1", "store-2"},
			"/store-1%2Cstore-2/_settings",
		},
	}

	for _, test := range tests {
		path, _, err := client.IndexPutSettings().Index(test.Indices...).buildURL()
		if err != nil {
			t.Fatal(err)
		}
		if path != test.Expected {
			t.Errorf("expected %q; got: %q", test.Expected, path)
		}
	}
}

func TestIndicesSettingsLifecycle(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	body := `{
		"index":{
			"refresh_interval":"-1"
		}
	}`

	// Put settings
	putres, err := client.IndexPutSettings().Index(testIndexName).BodyString(body).Do()
	if err != nil {
		t.Fatalf("expected put settings to succeed; got: %v", err)
	}
	if putres == nil {
		t.Fatalf("expected put settings response; got: %v", putres)
	}
	if !putres.Acknowledged {
		t.Fatalf("expected put settings ack; got: %v", putres.Acknowledged)
	}

	// Read settings
	getres, err := client.IndexGetSettings().Index(testIndexName).Do()
	if err != nil {
		t.Fatalf("expected get mapping to succeed; got: %v", err)
	}
	if getres == nil {
		t.Fatalf("expected get mapping response; got: %v", getres)
	}

	// Check settings
	index, found := getres[testIndexName]
	if !found {
		t.Fatalf("expected to return settings for index %q; got: %#v", testIndexName, getres)
	}
	// Retrieve "index" section of the settings for index testIndexName
	sectionIntf, ok := index.Settings["index"]
	if !ok {
		t.Fatalf("expected settings to have %q field; got: %#v", "index", getres)
	}
	section, ok := sectionIntf.(map[string]interface{})
	if !ok {
		t.Fatalf("expected settings to be of type map[string]interface{}; got: %#v", getres)
	}
	refintv, ok := section["refresh_interval"]
	if !ok {
		t.Fatalf(`expected JSON to include "refresh_interval" field; got: %#v`, getres)
	}
	if got, want := refintv, "-1"; got != want {
		t.Fatalf("expected refresh_interval = %v; got: %v", want, got)
	}
}
