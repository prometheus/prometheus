package index

import (
	"github.com/matttproud/prometheus/coding"
)

type MembershipIndex interface {
	Has(key coding.Encoder) (bool, error)
	Put(key coding.Encoder) error
	Drop(key coding.Encoder) error
	Close() error
}
