package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/storage/remote"
	"os"
	"path/filepath"
	"time"
)

type writeLoop struct {
	destination *Destination
	catalog     Catalog
}

func newWriteLoop(destination *Destination, catalog Catalog) *writeLoop {
	return &writeLoop{
		destination: destination,
		catalog:     catalog,
	}
}

func (wl *writeLoop) run(ctx context.Context) {
	var delay time.Duration
	var err error
	var client remote.WriteClient
	var ds *dataSource

	defer func() {
		if ds != nil {
			_ = ds.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			delay = 0
		}

		if client == nil {
			client, err = createClient(wl.destination.Config())
			if err != nil {
				logger.Errorf("failed to create client: %w", err)
				delay = time.Second * 5
				continue
			}
		}

		if ds == nil {
			ds, err = wl.nextDataSource(ctx)
			if err != nil {
				logger.Errorf("failed to get next data source: %w", err)
				delay = time.Second * 5
				continue
			}
		}

		if err = wl.write(ctx, ds, client); err != nil {
			logger.Errorf("failed to write data source: %w", err)
			delay = time.Second * 5
			continue
		}

		if err = ds.Close(); err != nil {
			logger.Errorf("failed to close data source: %w", err)
			delay = time.Second * 5
			continue
		}

		if err = ds.WriteCompletion(); err != nil {
			logger.Errorf("failed to write data source completion: %w", err)
			delay = time.Second * 5
			continue
		}

		ds = nil
	}
}

func createClient(config DestinationConfig) (client remote.WriteClient, err error) {
	clientConfig := remote.ClientConfig{
		URL:              config.URL,
		Timeout:          config.RemoteTimeout,
		HTTPClientConfig: config.HTTPClientConfig,
		SigV4Config:      config.SigV4Config,
		AzureADConfig:    config.AzureADConfig,
		Headers:          config.Headers,
		RetryOnRateLimit: true,
	}

	client, err = remote.NewWriteClient(config.Name, &clientConfig)
	if err != nil {
		return nil, fmt.Errorf("falied to create client: %w", err)
	}

	return client, nil
}

func (wl *writeLoop) write(ctx context.Context, ds *dataSource, client remote.WriteClient) error {
	var err error
	var delay time.Duration

	// todo: make scaler
	b := &batch{capacity: wl.destination.Config().QueueConfig.MaxSamplesPerSend * ds.NumberOfShards()}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay = 0
		}

		if err = wl.iterate(ctx, ds, b, newWriter(client, cppbridge.NewWALProtobufEncoder(ds.LSSes()))); err != nil {
			if errors.Is(err, ErrEndOfBlock) {
				return nil
			}
			if errors.Is(err, ErrNoData) {
				delay = time.Second * 5
				continue
			}
			if errors.Is(err, ErrBlockIsCorrupted) {
				return err
			}

			logger.Errorf("iteration failed: %w", err)
			delay = time.Second * 5
			continue
		}
	}
}

type Writer interface {
	Write(ctx context.Context, numberOfShards int, samples []*cppbridge.DecodedRefSamples) error
}

func (wl *writeLoop) iterate(ctx context.Context, ds *dataSource, b *batch, w Writer) error {
	var delay time.Duration
	var endOfBlockReached bool

	var targetSegmentID uint32
	if ds.lastAcknowledgedSegmentID != nil {
		targetSegmentID = *ds.lastAcknowledgedSegmentID + 1
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay = 0
		}

		if b.HasDataToSend() {
			data, segmentID := b.DataToSend()
			if err := w.Write(ctx, ds.NumberOfShards(), data); err != nil {
				return fmt.Errorf("write failed: %w", err)
			}
			if err := ds.Ack(segmentID); err != nil {
				return fmt.Errorf("failed to ack: %w", err)
			}
			b.Reset()
			return nil
		} else {
			if endOfBlockReached {
				return ErrEndOfBlock
			}
		}

		if !endOfBlockReached {
			samples, err := ds.Read(ctx, targetSegmentID)
			if err != nil {
				if errors.Is(err, ErrPartialReadResult) {
					if len(samples) > 0 {
						b.Add(targetSegmentID, samples)
					}
					delay = time.Second * 5
					continue
				}
				if errors.Is(err, ErrEmptyReadResult) {
					delay = time.Second * 5
					continue
				}
				if errors.Is(err, ErrEndOfBlock) {
					endOfBlockReached = true
					continue
				}

				logger.Errorf("unexpected block read error: %w", err)
				delay = time.Second * 5
				continue
			}

			b.Add(targetSegmentID, samples)
			targetSegmentID++
		}
	}
}

func (wl *writeLoop) nextDataSource(ctx context.Context) (*dataSource, error) {
	var nextHeadRecord catalog.Record
	var err error
	if wl.destination.HeadID != nil {
		nextHeadRecord, err = nextHead(ctx, wl.catalog, *wl.destination.HeadID)
	} else {
		nextHeadRecord, err = scanForNextHead(ctx, wl.catalog, wl.destination.Config().Name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find next head: %w", err)
	}

	crw, err := NewCursorReadWriter(filepath.Join(nextHeadRecord.Dir, fmt.Sprintf("%s.cursor", wl.destination.Config().Name)))
	if err != nil {
		return nil, fmt.Errorf("failed to create cursor: %w", err)
	}

	cursor, err := crw.Read()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to read cursor: %w", err), crw.Close())
	}

	crc32, err := wl.destination.Config().CRC32()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to calculate crc32: %w", err), crw.Close())
	}

	var discardCache bool
	if cursor.ConfigCRC32 == nil {
		if err = crw.Write(nil, &crc32); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to write crc32: %w", err), crw.Close())
		}
	} else {
		discardCache = *cursor.ConfigCRC32 != crc32
	}

	b, err := newBlock(nextHeadRecord.Dir, nextHeadRecord.NumberOfShards, wl.destination.Config(), cursor.SegmentID, discardCache)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to create data source: %w", err), crw.Close())
	}

	b.ID = nextHeadRecord.ID

	return &dataSource{
		block:                     b,
		crw:                       crw,
		lastAcknowledgedSegmentID: cursor.SegmentID,
		configCRC32:               crc32,
	}, nil
}

func nextHead(ctx context.Context, headCatalog Catalog, headID string) (catalog.Record, error) {
	if err := contextErr(ctx); err != nil {
		return catalog.Record{}, err
	}

	headRecords, err := headCatalog.List(
		nil,
		func(lhs, rhs catalog.Record) bool {
			return lhs.CreatedAt < rhs.CreatedAt
		},
	)
	if err != nil {
		return catalog.Record{}, fmt.Errorf("failed to list head records: %w", err)
	}

	if len(headRecords) == 0 {
		return catalog.Record{}, fmt.Errorf("no new heads")
	}

	for index, headRecord := range headRecords {
		if headRecord.ID == headID {
			if index == len(headRecords)-1 {
				return catalog.Record{}, fmt.Errorf("no new heads")
			}

			return headRecords[index+1], nil
		}
	}

	// unknown head id, selecting last head
	return headRecords[len(headRecords)-1], nil
}

func scanForNextHead(ctx context.Context, headCatalog Catalog, destinationName string) (catalog.Record, error) {
	if err := contextErr(ctx); err != nil {
		return catalog.Record{}, err
	}

	headRecords, err := headCatalog.List(
		nil,
		func(lhs, rhs catalog.Record) bool {
			return lhs.CreatedAt > rhs.CreatedAt
		},
	)
	if err != nil {
		return catalog.Record{}, fmt.Errorf("failed to list head records: %w", err)
	}

	if len(headRecords) == 0 {
		return catalog.Record{}, fmt.Errorf("no new heads")
	}

	var headFound bool
	for _, headRecord := range headRecords {
		headFound, err = scanHeadForDestination(headRecord, destinationName)
		if err != nil {
			return headRecord, err
		}
		if headFound {
			return headRecord, nil
		}
	}

	// track of the previous destination not found, selecting last head
	return headRecords[len(headRecords)-1], nil
}

func scanHeadForDestination(headRecord catalog.Record, destinationName string) (bool, error) {
	dir, err := os.Open(headRecord.Dir)
	if err != nil {
		return false, fmt.Errorf("failed to open head dir: %w", err)
	}
	defer func() { _ = dir.Close() }()

	fileNames, err := dir.Readdirnames(-1)
	if err != nil {
		return false, fmt.Errorf("failed to read dir names: %w", err)
	}

	for _, fileName := range fileNames {
		if fileName == fmt.Sprintf("%s.cursor", destinationName) {
			return true, nil
		}
	}

	return false, nil
}

//	type batch struct {
//		limit    int
//		segments []SegmentedDecodedRefSamples
//	}
//
//	func (b *batch) Add(segmentID uint32, shardedData []*cppbridge.DecodedRefSamples) {
//		b.segments = append(b.segments, SegmentedDecodedRefSamples{
//			SegmentID:   segmentID,
//			ShardedData: shardedData,
//		})
//	}
//
//	func (b *batch) Size() (size int) {
//		for _, segment := range b.segments {
//			for _, samples := range segment.ShardedData {
//				size += samples.Size()
//			}
//		}
//		return size
//	}
//
//	func (b *batch) DataToSend() ([]*cppbridge.DecodedRefSamples, uint32) {
//		var data []*cppbridge.DecodedRefSamples
//		var lastSegmentID uint32
//		var numberOfSamples int
//		for index, segment := range b.segments {
//			var nextSegmentSize int
//			if index < len(b.segments)-1 {
//				nextSegmentSize = func(segment SegmentedDecodedRefSamples) int {
//					size := 0
//					for _, samples := range segment.ShardedData {
//						size += samples.Size()
//					}
//					return size
//				}(segment)
//			}
//
//			if index != 0 && numberOfSamples+nextSegmentSize > b.limit {
//				return data, lastSegmentID
//			}
//
//			for _, samples := range segment.ShardedData {
//				numberOfSamples += samples.Size()
//				data = append(data, samples)
//			}
//
//			lastSegmentID = segment.SegmentID
//		}
//
//		return data, lastSegmentID
//	}
func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

//
//type RetryableError struct {
//	err error
//}
//
//func IsRetryableError(err error) bool {
//	return errors.As(err, &RetryableError{})
//}
//
//func NewRetryableError(err error) RetryableError {
//	return RetryableError{err: err}
//}
//
//func (e RetryableError) Error() string {
//	return e.err.Error()
//}
//
//func (e RetryableError) Cause() error {
//	return e.err
//}
