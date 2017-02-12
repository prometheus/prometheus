package warp10

import (
	"github.com/prometheus/common/model"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClient(t *testing.T) {
	samples := model.Samples{
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"test_label":          "test_label_value1",
			},
			Timestamp: model.Time(123456789123),
			Value:     1.23,
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"test_label":          "test_label_value2",
			},
			Timestamp: model.Time(123456789123),
			Value:     5.1234,
		},
	}
	token := "RANDOM_TOKEN"
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Fatalf("Unexpected method, expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/api/v0/update" {
				t.Fatalf("Unexpected path, expected /api/v0/update, got %s", r.URL.Path)
			}
			if _, ok := r.Header["X-Warp10-Token"]; ok == false {
				t.Fatalf("Header X-Warp10-Token must be set")
			}
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Error reading body: %s", err)
			}
			t.Logf("%s", body)

		},
	))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Unable to parse server URL %s: %s", server.URL, err)
	}
	client := NewClient(serverURL.Scheme+"://"+serverURL.Host+"/api/v0/update", token)
	client.Store(samples)
}
