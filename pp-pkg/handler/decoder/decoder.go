package decoder

import (
	"context"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// Decoder implements decoders.
type Decoder interface {
	DecodeToHashdex(ctx context.Context, segment model.Segment) (cppbridge.HashdexContent, error)
	Discard() error
	Close() error
}
