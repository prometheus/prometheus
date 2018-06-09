package main

import "testing"

func TestURLJoin(t *testing.T) {

	testCases := []struct {
		inputHost string
		inputPath string
		expected  string
	}{
		{"http://host", "path", "http://host/path"},
		{"http://host", "path/", "http://host/path"},
		{"http://host", "/path", "http://host/path"},
		{"http://host", "/path/", "http://host/path"},

		{"http://host/", "path", "http://host/path"},
		{"http://host/", "path/", "http://host/path"},
		{"http://host/", "/path", "http://host/path"},
		{"http://host/", "/path/", "http://host/path"},

		{"https://host", "path", "https://host/path"},
		{"https://host", "path/", "https://host/path"},
		{"https://host", "/path", "https://host/path"},
		{"https://host", "/path/", "https://host/path"},

		{"https://host/", "path", "https://host/path"},
		{"https://host/", "path/", "https://host/path"},
		{"https://host/", "/path", "https://host/path"},
		{"https://host/", "/path/", "https://host/path"},
	}
	for i, c := range testCases {
		client, err := NewHTTPClient(HTTPClientConfig{ServerURL: c.inputHost})
		if err != nil {
			panic(err)
		}
		actual := client.URLJoin(c.inputPath)
		if actual != c.expected {
			t.Errorf("Error on case %d: %v(actual) != %v(expected)", i, actual, c.expected)
		}
		t.Logf("Case %d: %v(actual) == %v(expected)", i, actual, c.expected)
	}
}
