package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/storage/remote"
	"os"
	"path/filepath"
	"time"
)

const defaultDelay = time.Second * 5

type writeLoop struct {
	destination *Destination
	catalog     Catalog
	clock       clockwork.Clock
}

func newWriteLoop(destination *Destination, catalog Catalog, clock clockwork.Clock) *writeLoop {
	return &writeLoop{
		destination: destination,
		catalog:     catalog,
		clock:       clock,
	}
}

func (wl *writeLoop) run(ctx context.Context) {
	var delay time.Duration
	var err error
	var client remote.WriteClient
	var i *Iterator

	defer func() {
		if i != nil {
			_ = i.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-wl.clock.After(delay):
			delay = 0
		}

		if client == nil {
			client, err = createClient(wl.destination.Config())
			if err != nil {
				logger.Errorf("failed to create client: %w", err)
				delay = defaultDelay
				continue
			}
		}

		if i == nil {
			i, err = wl.nextIterator(ctx, newWriter(client))
			if err != nil {
				logger.Errorf("failed to get next data source: %w", err)
				delay = defaultDelay
				continue
			}
		}

		if err = wl.write(ctx, i); err != nil {
			logger.Errorf("failed to write data source: %w", err)
			delay = defaultDelay
			continue
		}

		if err = i.Close(); err != nil {
			logger.Errorf("failed to close data source: %w", err)
			delay = defaultDelay
			continue
		}

		i = nil
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

func (wl *writeLoop) write(ctx context.Context, iterator *Iterator) error {
	var err error
	var delay time.Duration

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wl.clock.After(delay):
			delay = 0
		}

		err = iterator.Next(ctx)
		if err != nil {
			if errors.Is(err, ErrEndOfBlock) {
				return nil
			}

			logger.Errorf("iteration failed: %w", err)
			delay = defaultDelay
			continue
		}
	}
}

func (wl *writeLoop) nextIterator(ctx context.Context, writer Writer) (*Iterator, error) {
	var nextHeadRecord *catalog.Record
	var err error
	if wl.destination.HeadID != nil {
		nextHeadRecord, err = nextHead(ctx, wl.catalog, *wl.destination.HeadID)
	} else {
		nextHeadRecord, err = scanForNextHead(ctx, wl.catalog, wl.destination.Config().Name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find next head: %w", err)
	}

	crw, err := NewCursorReadWriter(filepath.Join(nextHeadRecord.Dir(), fmt.Sprintf("%s.cursor", wl.destination.Config().Name)))
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

	b, err := newBlock(nextHeadRecord.Dir(), nextHeadRecord.NumberOfShards(), wl.destination.Config(), cursor.SegmentID, discardCache, wl.makeCorruptMarker(), nextHeadRecord)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to create data source: %w", err), crw.Close())
	}

	b.ID = nextHeadRecord.ID()

	return newIterator(wl.clock, wl.destination.Config().QueueConfig, b, writer, newAcknowledger(crw, crc32), cursor.SegmentID, time.Second*30), nil
}

type CorruptMarkerFn func(headID string) error

func (fn CorruptMarkerFn) MarkCorrupted(headID string) error {
	return fn(headID)
}

func (wl *writeLoop) makeCorruptMarker() CorruptMarker {
	return CorruptMarkerFn(func(headID string) error {
		_, err := wl.catalog.SetStatus(headID, catalog.StatusCorrupted)
		return err
	})
}

func nextHead(ctx context.Context, headCatalog Catalog, headID string) (*catalog.Record, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}

	headRecords, err := headCatalog.List(
		nil,
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list head records: %w", err)
	}

	if len(headRecords) == 0 {
		return nil, fmt.Errorf("no new heads")
	}

	for index, headRecord := range headRecords {
		if headRecord.ID() == headID {
			if index == len(headRecords)-1 {
				return nil, fmt.Errorf("no new heads")
			}

			return headRecords[index+1], nil
		}
	}

	// unknown head id, selecting last head
	return headRecords[len(headRecords)-1], nil
}

func scanForNextHead(ctx context.Context, headCatalog Catalog, destinationName string) (*catalog.Record, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}

	headRecords, err := headCatalog.List(
		nil,
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() > rhs.CreatedAt()
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list head records: %w", err)
	}

	if len(headRecords) == 0 {
		return nil, fmt.Errorf("no new heads")
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

func scanHeadForDestination(headRecord *catalog.Record, destinationName string) (bool, error) {
	dir, err := os.Open(headRecord.Dir())
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

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
