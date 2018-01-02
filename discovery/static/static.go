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
