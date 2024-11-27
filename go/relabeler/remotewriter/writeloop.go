package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/storage/remote"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type writeLoop struct {
	destination *Destination
	catalog     Catalog
	client      remote.WriteClient
}

func newWriteLoop(destination *Destination, catalog Catalog) *writeLoop {
	return &writeLoop{
		destination: destination,
		catalog:     catalog,
	}
}

func (wl *writeLoop) run(ctx context.Context) {
	var ds *dataSource
	var cursor *Cursor
	var nextDs *dataSource
	var nextCursor *Cursor
	var err error

	defer func() {
		_ = CloseAll(ds, cursor, nextDs, nextCursor)
	}()

	var delay time.Duration

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			delay = 0
		}

		if (ds == nil && nextDs == nil) || (ds != nil && ds.IsCompleted() && nextDs == nil) {
			nextDs, nextCursor, err = wl.nextDataSource(ctx)
			if err != nil {
				logger.Errorf("failed to get next data source: %w", err)
				delay = time.Second * 5
				continue
			}
		}

		if ds == nil {
			ds = nextDs
			cursor = nextCursor
			nextDs = nil
			nextCursor = nil
		} else {
			if err = ds.Close(); err != nil {
				logger.Errorf("failed to close data source: %w", err)
				delay = time.Second * 5
				continue
			}
			if err = cursor.Close(); err != nil {
				logger.Errorf("failed to close cursor: %w", err)
				delay = time.Second * 5
				continue
			}
			if ds.IsCompleted() {
				if err = ds.WriteCompletion(); err != nil {
					logger.Errorf("failed to write data source completion: %w", err)
					delay = time.Second * 5
					continue
				}
				ds = nextDs
				cursor = nextCursor
				nextDs = nil
				nextCursor = nil
			}
		}

		if err = wl.write(ctx, ds, cursor); err != nil {

		}
	}
}

func (wl *writeLoop) write(ctx context.Context, ds *dataSource, cursor *Cursor) error {
	var err error
	var delay time.Duration
	b := &batch{
		limit: 5000,
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay = 0
		}

		err = wl.iterate(ctx, ds, cursor, b)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			if IsRetryableError(err) {
				delay = time.Second * 5
				continue
			}

			// todo: corrupted?
			return err
		}
	}
}

func (wl *writeLoop) nextDataSource(ctx context.Context) (*dataSource, *Cursor, error) {
	var nextHeadRecord catalog.Record
	var err error
	if wl.destination.HeadID != nil {
		nextHeadRecord, err = nextHead(ctx, wl.catalog, *wl.destination.HeadID)
	} else {
		nextHeadRecord, err = scanForNextHead(ctx, wl.catalog, wl.destination.Config().Name)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find next head: %w", err)
	}

	cursor, err := NewCursor(filepath.Join(nextHeadRecord.Dir, fmt.Sprintf("%s.cursor", wl.destination.Config().Name)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cursor: %w", err)
	}

	cursorData, err := cursor.Read()
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("failed to read cursor: %w", err), cursor.Close())
	}

	crc32, err := wl.destination.Config().CRC32()
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("failed to calculate crc32: %w", err), cursor.Close())
	}

	discardCache := cursorData.ConfigCRC32 != crc32

	ds, err := newDataSource(nextHeadRecord.Dir, nextHeadRecord.NumberOfShards, wl.destination.Config(), discardCache)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("failed to create data source: %w", err), cursor.Close())
	}

	ds.ID = nextHeadRecord.ID

	return ds, cursor, nil
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
			return lhs.CreatedAt < rhs.CreatedAt
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

func (wl *writeLoop) iterate(ctx context.Context, ds *dataSource, cursor *Cursor, b *batch) error {
	timer := time.NewTimer(time.Second * 30)
	defer timer.Stop()

	var data []*cppbridge.DecodedRefSamples
	var segmentID uint32
	var err error
	var delay time.Duration

	for {

		select {
		case <-timer.C:
			if b.Size() == 0 {
				return nil
			}

			// todo: send
		case <-time.After(delay):
			delay = 0
		}

		if ds.completed {
			// todo: send and exit
		}

		data, segmentID, err = ds.Next(ctx)
		if err != nil {
			//if errors.Is(err, )
		}

		b.Add(segmentID, data)
	}

	return ds.Ack(segmentID)
}

func (wl *writeLoop) send(ctx context.Context, data []*cppbridge.DecodedRefSamples, lsses []*cppbridge.LabelSetStorage) error {
	encoder := cppbridge.NewWALProtobufEncoder(lsses)
	shardedProtobuf, err := encoder.Encode(ctx, data, 5)
	if err != nil {
		// todo: unretryable error
		return fmt.Errorf("failed to encode protobuf: %w", err)
	}

	if wl.client == nil {
		config := wl.destination.Config()
		clientConfig := remote.ClientConfig{
			URL:              config.URL,
			Timeout:          config.RemoteTimeout,
			HTTPClientConfig: config.HTTPClientConfig,
			SigV4Config:      config.SigV4Config,
			AzureADConfig:    config.AzureADConfig,
			Headers:          config.Headers,
			RetryOnRateLimit: true,
		}
		wl.client, err = remote.NewWriteClient(config.Name, &clientConfig)
		if err != nil {
			// todo: retryable error
			return fmt.Errorf("falied to create client: %w", err)
		}

	}

	wg := &sync.WaitGroup{}
	errs := make([]error, len(shardedProtobuf))
	for shardID, protobuf := range shardedProtobuf {
		wg.Add(1)
		go func(client remote.WriteClient, protobuf *cppbridge.SnappyProtobufEncodedData, shardID int) {
			defer wg.Done()
			err = protobuf.Do(func(buf []byte) error {
				return client.Store(ctx, buf, 10)
			})
			errs[shardID] = err
		}(wl.client, protobuf, shardID)
	}

	return errors.Join(err)
}

type batch struct {
	limit    int
	segments []SegmentedDecodedRefSamples
}

func (b *batch) Add(segmentID uint32, shardedData []*cppbridge.DecodedRefSamples) {
	b.segments = append(b.segments, SegmentedDecodedRefSamples{
		SegmentID:   segmentID,
		ShardedData: shardedData,
	})
}

func (b *batch) Size() (size int) {
	for _, segment := range b.segments {
		for _, samples := range segment.ShardedData {
			size += samples.Size()
		}
	}
	return size
}

func (b *batch) DataToSend() ([]*cppbridge.DecodedRefSamples, uint32) {
	var data []*cppbridge.DecodedRefSamples
	var lastSegmentID uint32
	var numberOfSamples int
	for index, segment := range b.segments {
		var nextSegmentSize int
		if index < len(b.segments)-1 {
			nextSegmentSize = func(segment SegmentedDecodedRefSamples) int {
				size := 0
				for _, samples := range segment.ShardedData {
					size += samples.Size()
				}
				return size
			}(segment)
		}

		if index != 0 && numberOfSamples+nextSegmentSize > b.limit {
			return data, lastSegmentID
		}

		for _, samples := range segment.ShardedData {
			numberOfSamples += samples.Size()
			data = append(data, samples)
		}

		lastSegmentID = segment.SegmentID
	}

	return data, lastSegmentID
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

type RetryableError struct {
	err error
}

func IsRetryableError(err error) bool {
	return errors.As(err, &RetryableError{})
}

func NewRetryableError(err error) RetryableError {
	return RetryableError{err: err}
}

func (e RetryableError) Error() string {
	return e.err.Error()
}

func (e RetryableError) Cause() error {
	return e.err
}

type SegmentedDecodedRefSamples struct {
	SegmentID   uint32
	ShardedData []*cppbridge.DecodedRefSamples
}

type ReadAcker struct {
	dataSource *dataSource
	cursor     *Cursor
}

func NewReadAcker(dataSource *dataSource, cursor *Cursor) *ReadAcker {
	return &ReadAcker{
		dataSource: dataSource,
		cursor:     cursor,
	}
}

func (ra *ReadAcker) Next(ctx context.Context) ([]*cppbridge.DecodedRefSamples, uint32, error) {
	return ra.dataSource.Next(ctx)
}

func (ra *ReadAcker) Ack(segmentID uint32) error {
	return ra.cursor.Write(CursorData{
		SegmentID:   segmentID,
		ConfigCRC32: 0,
	})
}

func (ra *ReadAcker) Close() error {
	return errors.Join(ra.dataSource.Close(), ra.cursor.Close())
}
