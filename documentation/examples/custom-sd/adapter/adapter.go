// Copyright 2018 The Prometheus Authors
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

package adapter

// NOTE: you do not need to edit this file when implementing a custom service discovery.
import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type customSD struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

// Adapter runs an unknown service discovery implementation and converts its target groups
// to JSON and writes to a file for file_sd.
type Adapter struct {
	ctx      context.Context
	provider discovery.Provider
	groups   map[string]*customSD
	manager  *discovery.Manager
	output   string
	logger   log.Logger
}

func generateTargetGroups(allTargetGroups map[string][]*targetgroup.Group) map[string]*customSD {
	groups := make(map[string]*customSD)
	for _, sdTargetGroups := range allTargetGroups {
		for _, group := range sdTargetGroups {
			groupLabels := make(map[string]string, len(group.Labels))
			for name, value := range group.Labels {
				groupLabels[string(name)] = string(value)
			}

			for _, target := range group.Targets {
				var (
					targetAddress string
					targetLabels  = make(map[string]string)
				)

				for ln, lv := range target {
					if ln == model.AddressLabel {
						targetAddress = string(lv)
						continue
					}
					targetLabels[string(ln)] = string(lv)
				}
				if targetAddress == "" {
					continue
				}
				for k, v := range groupLabels {
					if _, ok := targetLabels[k]; !ok {
						targetLabels[k] = v
					}
				}
				sdGroup := customSD{
					Targets: []string{targetAddress},
					Labels:  targetLabels,
				}
				// Make a unique key with the target's fingerprint.
				fp := target.Fingerprint() ^ group.Labels.Fingerprint()
				groups[fp.String()] = &sdGroup
			}
		}
	}

	return groups
}

// Parses the incoming target groups updates. If the update contains changes to the
// target groups which the adapter already knows about, or new target groups,
// we marshal to JSON and write to the output file.
func (a *Adapter) refreshTargetGroups(allTargetGroups map[string][]*targetgroup.Group) {
	tempGroups := generateTargetGroups(allTargetGroups)

	if _, err := os.Stat(a.output); err == nil && reflect.DeepEqual(a.groups, tempGroups) {
		return
	}

	level.Debug(a.logger).Log("msg", "updating output file")
	a.groups = tempGroups
	err := a.writeOutput()
	if err != nil {
		level.Error(a.logger).Log("msg", "failed to write output", "err", err)
	}
}

func (a *Adapter) serialize(w io.Writer) error {
	var (
		arr  = make([]*customSD, 0, len(a.groups))
		keys = make([]string, 0, len(a.groups))
	)
	for k := range a.groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		arr = append(arr, a.groups[k])
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(arr)
}

// Writes JSON formatted targets to the output file.
func (a *Adapter) writeOutput() error {
	dir := filepath.Dir(a.output)
	tmpfile, err := ioutil.TempFile(dir, "sd-adapter")
	if err != nil {
		return err
	}
	defer tmpfile.Close()

	err = a.serialize(tmpfile)
	if err != nil {
		return err
	}

	return os.Rename(tmpfile.Name(), a.output)
}

// Run starts the custom service discovery implementation. It returns only when
// the context is done.
func (a *Adapter) Run() {
	a.manager.ApplyConfig([]discovery.Provider{a.provider})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-a.ctx.Done():
				return
			case allTargetGroups, ok := <-a.manager.SyncCh():
				// Handle the case that a target provider exits and closes the channel
				// before the context is done.
				if !ok {
					return
				}
				a.refreshTargetGroups(allTargetGroups)
			}
		}
	}()

	a.manager.Run()
	<-done
}

// NewAdapter creates a new instance of Adapter.
func NewAdapter(ctx context.Context, file string, d discovery.Discoverer, logger log.Logger) *Adapter {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &Adapter{
		ctx:      ctx,
		provider: discovery.NewProvider("sd-adapter", d),
		groups:   make(map[string]*customSD),
		manager:  discovery.NewManager(ctx, logger),
		output:   file,
		logger:   log.With(logger, "component", "sd-adapter"),
	}
}
