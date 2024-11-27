package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/model/labels"
	"io"
	"os"
)

type Decoder struct {
	relabeler     *cppbridge.StatelessRelabeler
	lss           *cppbridge.LabelSetStorage
	outputDecoder *cppbridge.WALOutputDecoder
	cache         *os.File
}

func NewDecoder(
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	cacheFileName string,
	discardCache bool,
	shardID uint16,
	encoderVersion uint8) (*Decoder, error) {
	relabeler, err := cppbridge.NewStatelessRelabeler(relabelConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create stateless relabeler: %w", err)
	}

	lss := cppbridge.NewLssStorage()
	outputDecoder := cppbridge.NewWALOutputDecoder(labelsToCppBridgeLabels(externalLabels), relabeler, lss, shardID, encoderVersion)
	cache, err := os.OpenFile(cacheFileName, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open cache file: %w", err)
	}

	if discardCache {
		if err = cache.Truncate(0); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to discard cache: %w", err), cache.Close())
		}
	} else {
		cacheData, err := io.ReadAll(cache)
		if err != nil {
			logger.Errorf("failed to read cache file: %w", err)
		}

		if err = outputDecoder.LoadFrom(cacheData); err != nil {
			logger.Errorf("failed to load from cache data: %w", err)
		}
	}

	d := &Decoder{
		relabeler:     relabeler,
		lss:           lss,
		outputDecoder: outputDecoder,
		cache:         cache,
	}

	return d, nil
}

func labelsToCppBridgeLabels(labels labels.Labels) []cppbridge.Label {
	return nil
}

func (d *Decoder) Decode(ctx context.Context, segment []byte) (*cppbridge.DecodedRefSamples, error) {
	return d.outputDecoder.Decode(ctx, segment)
}

func (d *Decoder) WriteCache() (err error) {
	if _, err = d.cache.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to set cache file offset: %w", err)
	}
	bytesWritten, err := d.outputDecoder.WriteTo(d.cache)
	if err = d.cache.Truncate(bytesWritten); err != nil {
		return fmt.Errorf("failed to truncate cache file: %w", err)
	}
	return nil
}
