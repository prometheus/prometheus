package remotewriter

import (
	"errors"
	"fmt"
	"os"

	"github.com/prometheus/prometheus/pp/go/relabeler/head"
)

type walReader struct {
	nextSegmentID uint32
	file          *os.File
}

func newWalReader(fileName string) (*walReader, uint8, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read wal file: %w", err)
	}

	_, encoderVersion, _, err := head.ReadHeader(file)
	if err != nil {
		return nil, 0, errors.Join(fmt.Errorf("failed to read header: %w", err), file.Close())
	}

	return &walReader{
		file: file,
	}, encoderVersion, nil
}

type Segment struct {
	ID             uint32
	encoderVersion uint8
	head.DecodedSegment
}

func (r *walReader) Read() (segment Segment, err error) {
	decodedSegment, _, err := head.ReadSegment(r.file)
	if err != nil {
		return segment, fmt.Errorf("failed to read segment: %w", err)
	}

	segment.ID = r.nextSegmentID
	r.nextSegmentID++
	segment.DecodedSegment = decodedSegment

	return segment, nil
}

func (r *walReader) Close() error {
	return r.file.Close()
}
