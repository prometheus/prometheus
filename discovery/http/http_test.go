// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRemoteFile(t *testing.T) {
	ch := make(chan []*targetgroup.Group)
	end := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()
	go func() {
		NewDiscovery(
			&SDConfig{
				Paths: []string{
					"https://raw.githubusercontent.com/prometheus/prometheus/master/discovery/file/fixtures/valid.json",
				},
				// Setting a high refresh interval to make sure that the test only
				// runs once
				RefreshInterval: model.Duration(1 * time.Hour),
			},
			nil,
		).Run(ctx, ch)
		end <- 1
	}()
	select {
	case remote := <-ch:
		b1, _ := json.Marshal(remote)
		test := "[{\"Targets\":[{\"__address__\":\"localhost:9090\"},{\"__address__\":\"example.org:443\"}],\"Labels\":{\"__meta_filepath\":\"https://raw.githubusercontent.com/prometheus/prometheus/master/discovery/file/fixtures/valid.json\",\"foo\":\"bar\"},\"Source\":\"https://raw.githubusercontent.com/prometheus/prometheus/master/discovery/file/fixtures/valid.json:0\"},{\"Targets\":[{\"__address__\":\"my.domain\"}],\"Labels\":{\"__meta_filepath\":\"https://raw.githubusercontent.com/prometheus/prometheus/master/discovery/file/fixtures/valid.json\"},\"Source\":\"https://raw.githubusercontent.com/prometheus/prometheus/master/discovery/file/fixtures/valid.json:1\"}]"
		if string(b1) != test {
			log.Fatal("Remote load mismatch")
		}
	case <-time.After(5 * time.Second):
		fmt.Println("Remote file test skipped")
	}
	cancel()
	<-end
}
