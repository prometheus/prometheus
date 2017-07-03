// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestPutWarmerBuildURL(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tests := []struct {
		Indices  []string
		Types    []string
		Name     string
		Expected string
	}{
		{
			[]string{},
			[]string{},
			"warmer_1",
			"/_warmer/warmer_1",
		},
		{
			[]string{"*"},
			[]string{},
			"warmer_1",
			"/%2A/_warmer/warmer_1",
		},
		{
			[]string{},
			[]string{"*"},
			"warmer_1",
			"/_all/%2A/_warmer/warmer_1",
		},
		{
			[]string{"index-1", "index-2"},
			[]string{"type-1", "type-2"},
			"warmer_1",
			"/index-1%2Cindex-2/type-1%2Ctype-2/_warmer/warmer_1",
		},
	}

	for _, test := range tests {
		path, _, err := client.PutWarmer().Index(test.Indices...).Type(test.Types...).Name(test.Name).buildURL()
		if err != nil {
			t.Fatal(err)
		}
		if path != test.Expected {
			t.Errorf("expected %q; got: %q", test.Expected, path)
		}
	}
}

func TestWarmerLifecycle(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	mapping := `{
		"query": {
			"match_all": {}
		}
	}`

	// Ensure well prepared test index
	client.Flush(testIndexName2).Do()

	putresp, err := client.PutWarmer().Index(testIndexName2).Type("tweet").Name("warmer_1").BodyString(mapping).Do()
	if err != nil {
		t.Fatalf("expected put warmer to succeed; got: %v", err)
	}
	if putresp == nil {
		t.Fatalf("expected put warmer response; got: %v", putresp)
	}
	if !putresp.Acknowledged {
		t.Fatalf("expected put warmer ack; got: %v", putresp.Acknowledged)
	}

	getresp, err := client.GetWarmer().Index(testIndexName2).Name("warmer_1").Do()
	if err != nil {
		t.Fatalf("expected get warmer to succeed; got: %v", err)
	}
	if getresp == nil {
		t.Fatalf("expected get warmer response; got: %v", getresp)
	}
	props, ok := getresp[testIndexName2]
	if !ok {
		t.Fatalf("expected JSON root to be of type map[string]interface{}; got: %#v", props)
	}

	delresp, err := client.DeleteWarmer().Index(testIndexName2).Name("warmer_1").Do()
	if err != nil {
		t.Fatalf("expected del warmer to succeed; got: %v", err)
	}
	if delresp == nil {
		t.Fatalf("expected del warmer response; got: %v", getresp)
	}
	if !delresp.Acknowledged {
		t.Fatalf("expected del warmer ack; got: %v", delresp.Acknowledged)
	}
}
