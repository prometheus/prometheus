package decoder

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"

	"github.com/prometheus/prometheus/op-pkg/handler/model"
)

// Decoder implements decoders.
type Decoder interface {
	DecodeToHashdex(ctx context.Context, segment model.Segment) (cppbridge.HashdexContent, error)
	Discard() error
	Close() error
}
