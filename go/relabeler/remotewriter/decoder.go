package remotewriter

import (
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/model/labels"
	"io"
)

type Decoder struct {
	relabeler     *cppbridge.StatelessRelabeler
	lss           *cppbridge.LabelSetStorage
	outputDecoder *cppbridge.WALOutputDecoder
}

func NewDecoder(
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	shardID uint16,
	encoderVersion uint8) (*Decoder, error) {
	relabeler, err := cppbridge.NewStatelessRelabeler(relabelConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create stateless relabeler: %w", err)
	}

	lss := cppbridge.NewLssStorage()
	outputDecoder := cppbridge.NewWALOutputDecoder(labelsToCppBridgeLabels(externalLabels), relabeler, lss, shardID, encoderVersion)

	d := &Decoder{
		relabeler:     relabeler,
		lss:           lss,
		outputDecoder: outputDecoder,
	}

	return d, nil
}

func labelsToCppBridgeLabels(labels labels.Labels) []cppbridge.Label {
	return nil
}

func (d *Decoder) Decode(segment []byte) (*cppbridge.DecodedRefSamples, error) {
	return d.outputDecoder.Decode(segment)
}

func (d *Decoder) LoadFrom(reader io.Reader) error {
	state, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read from reader: %w", err)
	}

	return d.outputDecoder.LoadFrom(state)
}

// WriteTo writes output decoder state to io.Writer.
func (d *Decoder) WriteTo(writer io.Writer) (int64, error) {
	return d.outputDecoder.WriteTo(writer)
}
