package url

import (
	"testing"
	"net/http/httptest"
	"net/http"
	"os"
	"io"
	"time"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"
	"context"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestValidDiscovery(t *testing.T) {

	ts := createUnstartedServer("fixtures/valid.json")
	ts.Start()
	defer ts.Close()

	d, _ := time.ParseDuration("2s")
	sdc := SDConfig{
		Url:             ts.URL,
		RefreshInterval: model.Duration(d),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wds := NewDiscovery(&sdc, log.NewNopLogger())
	ch := make(chan []*targetgroup.Group)

	go wds.Run(ctx, ch)

	select {
	case res := <-ch:
		testutil.Equals(t, 2, len(res))
	case <-time.After(3 * time.Millisecond):
		t.Fatal("target groups discovery timeout")
	}
}

func TestNullTargetDiscovery(t *testing.T) {

	ts := createUnstartedServer("fixtures/invalid_nil.json")
	ts.Start()
	defer ts.Close()

	d, _ := time.ParseDuration("2s")
	sdc := SDConfig{
		Url:             ts.URL,
		RefreshInterval: model.Duration(d),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wds := NewDiscovery(&sdc, log.NewNopLogger())
	ch := make(chan []*targetgroup.Group)

	go wds.Run(ctx, ch)

	select {
	case <-ch:
		t.Fatal("unwanted target group results")
	case <-time.After(3 * time.Second):
	}
}

func TestPeriodicDiscovery(t *testing.T) {

	ts := createUnstartedServer("fixtures/valid.json")
	defer ts.Close()

	d, _ := time.ParseDuration("1s")
	sdc := SDConfig{
		Url:             ts.URL,
		RefreshInterval: model.Duration(d),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wds := NewDiscovery(&sdc, log.NewNopLogger())
	ch := make(chan []*targetgroup.Group)

	go wds.Run(ctx, ch)

	select {
	case <-time.After(2 * time.Second):
		ts.Start()
	case <-time.After(3 * time.Second):
		testutil.Equals(t, 2, len(<-ch))
	}
}

func createUnstartedServer(fixture string) (ts *httptest.Server) {

	return httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		r, _ := os.Open(fixture)
		io.Copy(writer, r)
	}))
}
