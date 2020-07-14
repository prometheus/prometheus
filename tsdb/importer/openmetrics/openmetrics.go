package openmetrics

import (
	"bufio"
	"io"

	"github.com/prometheus/prometheus/pkg/textparse"
)

// Content Type for the Open Metrics Parser.
// Needed to init the text parser.
const contentType = "application/openmetrics-text;"

type Parser struct {
	textparse.Parser

	s *bufio.Scanner
}

// NewParser is a tiny layer between textparse.Parser and importer.Parser which works on io.Reader.
// TODO(bwplotka): This is hacky, Parser should allow passing reader from scratch.
func NewParser(r io.Reader) textparse.Parser {
	// TODO(dipack95): Maybe use bufio.Reader.Readline instead?
	// https://stackoverflow.com/questions/21124327/how-to-read-a-text-file-line-by-line-in-go-when-some-lines-are-long-enough-to-ca
	return &Parser{s: bufio.NewScanner(r)}
}

// Next advances the parser to the next sample. It returns io.EOF if no
// more samples were read.
// TODO(bwplotka): Rought implementation, not tested, please help dipack95! (:
func (p *Parser) Next() (textparse.Entry, error) {
	for p.s.Scan() {
		// TODO(bwplotka): Assuming all line by line. If not do refetch like in previous version with more lines.
		line := p.s.Bytes()

		p.Parser = textparse.New(line, contentType)
		if et, err := p.Parser.Next(); err != io.EOF {
			return et, err
		}
		// EOF from parser, continue scanning.
	}
	if err := p.s.Err(); err != nil {
		return 0, err
	}
	return 0, io.EOF
}

// Series returns the bytes of the series, the timestamp if set, and the value
// of the current sample. Timestamps is milliseconds.
func (p *Parser) Series() ([]byte, *int64, float64) {
	b, ts, v := p.Parser.Series()
	if ts != nil {
		// OpenMetrics parser has time in nanoseconds. Convert to milliseconds.
		*ts = *ts / 1000
	}
	return b, ts, v
}
