package discovery

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

func TestFileSD(t *testing.T) {
	defer os.Remove("fixtures/_test.yml")
	defer os.Remove("fixtures/_test.json")
	testFileSD(t, ".yml")
	testFileSD(t, ".json")
}

func testFileSD(t *testing.T, ext string) {
	// As interval refreshing is more of a fallback, we only want to test
	// whether file watches work as expected.
	var conf config.FileSDConfig
	conf.Names = []string{"fixtures/_*" + ext}
	conf.RefreshInterval = model.Duration(1 * time.Hour)

	var (
		fsd  = NewFileDiscovery(&conf)
		ch   = make(chan config.TargetGroup)
		done = make(chan struct{})
	)
	go fsd.Run(ch, done)

	select {
	case <-time.After(25 * time.Millisecond):
		// Expected.
	case tg := <-ch:
		t.Fatalf("Unexpected target group in file discovery: %s", tg)
	}

	newf, err := os.Create("fixtures/_test" + ext)
	if err != nil {
		t.Fatal(err)
	}
	defer newf.Close()

	f, err := os.Open("fixtures/target_groups" + ext)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = io.Copy(newf, f)
	if err != nil {
		t.Fatal(err)
	}
	newf.Close()

	// The files contain two target groups which are read and sent in order.
	select {
	case <-time.After(15 * time.Second):
		t.Fatalf("Expected new target group but got none")
	case tg := <-ch:
		if _, ok := tg.Labels["foo"]; !ok {
			t.Fatalf("Label not parsed")
		}
		if tg.String() != fmt.Sprintf("fixtures/_test%s:0", ext) {
			t.Fatalf("Unexpected target group %s", tg)
		}
	}
	select {
	case <-time.After(15 * time.Second):
		t.Fatalf("Expected new target group but got none")
	case tg := <-ch:
		if tg.String() != fmt.Sprintf("fixtures/_test%s:1", ext) {
			t.Fatalf("Unexpected target group %s", tg)
		}
	}
	// Based on unknown circumstances, sometimes fsnotify will trigger more events in
	// some runs (which might be empty, chains of different operations etc.).
	// We have to drain those (as the target manager would) to avoid deadlocking and must
	// not try to make sense of it all...
	drained := make(chan struct{})
	go func() {
		for tg := range ch {
			// Below we will change the file to a bad syntax. Previously extracted target
			// groups must not be deleted via sending an empty target group.
			if len(tg.Targets) == 0 {
				t.Errorf("Unexpected empty target group received: %s", tg)
			}
		}
		close(drained)
	}()

	newf, err = os.Create("fixtures/_test.new")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(newf.Name())

	if _, err := newf.Write([]byte("]gibberish\n][")); err != nil {
		t.Fatal(err)
	}
	newf.Close()

	os.Rename(newf.Name(), "fixtures/_test"+ext)

	close(done)
	<-drained
}
