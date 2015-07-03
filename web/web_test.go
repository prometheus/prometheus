package web

import (
	"net/url"
	"testing"
)

func TestGlobalURL(t *testing.T) {
	opts := &Options{
		ListenAddress: ":9090",
		ExternalURL: &url.URL{
			Scheme: "https",
			Host:   "externalhost:80",
			Path:   "/path/prefix",
		},
	}

	tests := []struct {
		inURL  string
		outURL string
	}{
		{
			// Nothing should change if the input URL is not on localhost, even if the port is our listening port.
			inURL:  "http://somehost:9090/metrics",
			outURL: "http://somehost:9090/metrics",
		},
		{
			// Port and host should change if target is on localhost and port is our listening port.
			inURL:  "http://localhost:9090/metrics",
			outURL: "https://externalhost:80/metrics",
		},
		{
			// Only the host should change if the port is not our listening port, but the host is localhost.
			inURL:  "http://localhost:8000/metrics",
			outURL: "http://externalhost:8000/metrics",
		},
		{
			// Alternative localhost representations should also work.
			inURL:  "http://127.0.0.1:9090/metrics",
			outURL: "https://externalhost:80/metrics",
		},
	}

	for i, test := range tests {
		inURL, err := url.Parse(test.inURL)
		if err != nil {
			t.Fatalf("%d. Error parsing input URL: %s", i, err)
		}
		globalURL := tmplFuncs("", opts)["globalURL"].(func(u *url.URL) *url.URL)
		outURL := globalURL(inURL)

		if outURL.String() != test.outURL {
			t.Fatalf("%d. got %s, want %s", i, outURL.String(), test.outURL)
		}
	}
}
