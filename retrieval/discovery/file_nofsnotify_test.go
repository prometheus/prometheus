// These arches do not have working implementations of fsnotify.
// +build linux
// +build arm64 ppc64le ppc64

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
	// Can only test interval refreshing.
	var conf config.FileSDConfig
	conf.Names = []string{"fixtures/_*" + ext}
	conf.RefreshInterval = model.Duration(5 * time.Second)

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
}
