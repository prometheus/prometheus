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

package eureka

import (
	"errors"
	"testing"

	"github.com/prometheus/common/log"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

var (
	testServers = []string{"http://localhost:8761/eureka"}
	conf        = config.EurekaSDConfig{Servers: testServers}
)

func testUpdateServices(ch chan []*config.TargetGroup) error {
	md, err := NewDiscovery(&conf, log.Base())
	if err != nil {
		return err
	}
	return md.updateServices(context.Background(), ch)
}

func TestMarathonSDHandleError(t *testing.T) {
	var (
		errTesting = errors.New("testing failure")
		ch         = make(chan []*config.TargetGroup, 1)
	)
	if err := testUpdateServices(ch); err != errTesting {
		t.Fatalf("Expected error: %s", err)
	}
	select {
	case tg := <-ch:
		t.Fatalf("Got group: %s", tg)
	default:
	}
}
