// Copyright 2013 The Prometheus Authors
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

package static

import (
	"context"
	"fmt"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Provider holds a list of target groups that never change.
type Provider struct {
	TargetGroups []*targetgroup.Group
}

// NewProvider returns a StaticProvider configured with the given
// target groups.
func NewProvider(groups []*targetgroup.Group) *Provider {
	for i, tg := range groups {
		tg.Source = fmt.Sprintf("%d", i)
	}
	return &Provider{groups}
}

// Run implements the Worker interface.
func (sd *Provider) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// We still have to consider that the consumer exits right away in which case
	// the context will be canceled.
	select {
	case ch <- sd.TargetGroups:
	case <-ctx.Done():
	}
	close(ch)
}
