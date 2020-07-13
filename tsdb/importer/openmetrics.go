package importer

import (
	"io"

	"github.com/prometheus/prometheus/pkg/textparse"
)

type OpenMetricsParser struct {
}

func (o OpenMetricsParser) IsParsable(reader io.Reader) error {
	panic("implement me")
}

func (o OpenMetricsParser) Parse(reader io.Reader) textparse.Parser {
	panic("implement me")
}
