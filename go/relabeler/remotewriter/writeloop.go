package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"io"
	"os"
	"time"
)

type writeLoop struct {
	destination *Destination
	catalog     Catalog
	stopc       chan struct{}
	stoppedc    chan struct{}
}

func newWriteLoop(destination *Destination, catalog Catalog) *writeLoop {
	return &writeLoop{
		destination: destination,
		catalog:     catalog,
		stopc:       make(chan struct{}),
		stoppedc:    make(chan struct{}),
	}
}

func (wl *writeLoop) start() {
	defer close(wl.stoppedc)
	var ds *dataSource
	var err error

	defer func() {
		if ds != nil {
			_ = ds.Close()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())

	var backOff time.Duration
	for {
		select {
		case <-wl.stopc:
			return
		case <-time.After(backOff):
			backOff = 0
		}

		var nextDs *dataSource
		nextDs, err = wl.nextDataSource()
		if err != nil {
			fmt.Println("failed to get next data source", err)
			backOff = time.Minute
			continue
		}

		if nextDs != ds {
			if ds != nil {
				if err = ds.Close(); err != nil {
					// todo: throw error
					return
				}
			}
			ds = nextDs
			nextDs = nil
		}

		var delay time.Duration
		var stopped bool
		var iterationFinished bool
	loop:
		for {

			if stopped && iterationFinished {
				cancel()
				return
			}

			select {
			case <-wl.stopc:
				stopped = true
				cancel()
			case <-time.After(delay):
				iterationFinished = false
				err = wl.iterate(ctx, ds)
				iterationFinished = true
				if err != nil {
					if errors.Is(err, io.EOF) {
						break loop
					}
					if errors.Is(err, ErrNoData) {
						delay = time.Second * 5
						continue
					}

					// todo: mark as corrupted
					break loop
				}
				delay = time.Second * 30
			}
		}
	}
}

func (wl *writeLoop) stop() {
	close(wl.stopc)
	<-wl.stoppedc
}

func (wl *writeLoop) nextDataSource() (*dataSource, error) {
	var nextHeadRecord catalog.Record
	var err error
	if wl.destination.HeadID != nil {
		nextHeadRecord, err = nextHead(wl.catalog, *wl.destination.HeadID)
	} else {
		nextHeadRecord, err = scanForNextHead(wl.catalog, wl.destination.Config().Name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find next head: %w", err)
	}

	ds, err := newDataSource(nextHeadRecord.Dir, nextHeadRecord.NumberOfShards, wl.destination.Config())
	if err != nil {
		return nil, fmt.Errorf("failed to create data source: %w", err)
	}

	ds.ID = nextHeadRecord.ID

	return ds, nil
}

func nextHead(headCatalog Catalog, headID string) (catalog.Record, error) {
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

func scanForNextHead(headCatalog Catalog, destinationName string) (catalog.Record, error) {
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

var ErrNoData = errors.New("no data")

func (wl *writeLoop) iterate(ctx context.Context, ds *dataSource) error {
	data, segmentID, err := ds.Next(ctx)
	if err != nil {
		return err
	}

	fmt.Println("data", len(data))
	return ds.Ack(segmentID)
}
