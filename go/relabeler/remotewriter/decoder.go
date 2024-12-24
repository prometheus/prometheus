package remotewriter

import (
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/model/labels"
	"io"
	"os"
)

type Decoder struct {
	relabeler     *cppbridge.StatelessRelabeler
	lss           *cppbridge.LabelSetStorage
	outputDecoder *cppbridge.WALOutputDecoder
	state         *os.File
}

func NewDecoder(
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	shardID uint16,
	encoderVersion uint8,
) (*Decoder, error) {
	relabeler, err := cppbridge.NewStatelessRelabeler(relabelConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create stateless relabeler: %w", err)
	}

	lss := cppbridge.NewLssStorage()
	outputDecoder := cppbridge.NewWALOutputDecoder(labelsToCppBridgeLabels(externalLabels), relabeler, lss, shardID, encoderVersion)

	return &Decoder{
		relabeler:     relabeler,
		lss:           lss,
		outputDecoder: outputDecoder,
	}, nil
}

func labelsToCppBridgeLabels(labels labels.Labels) []cppbridge.Label {
	result := make([]cppbridge.Label, 0, len(labels))
	for _, label := range labels {
		result = append(result, cppbridge.Label{
			Name:  label.Name,
			Value: label.Value,
		})
	}
	return result
}

type DecodedSegment struct {
	ID                   uint32
	Samples              *cppbridge.DecodedRefSamples
	MaxTimestamp         int64
	OutdatedSamplesCount uint64
	DroppedSamplesCount  uint64
}

func (d *Decoder) Decode(segment []byte, minTimestamp int64) (*DecodedSegment, error) {
	samples, stats, err := d.outputDecoder.Decode(segment, minTimestamp)
	if err != nil {
		return nil, err
	}
	return &DecodedSegment{
		Samples:             samples,
		MaxTimestamp:        stats.MaxTimestamp(),
		DroppedSamplesCount: stats.DroppedSampleCount(),
	}, nil
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
