package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/prometheus/prometheus/pp/go/delivery"
	"github.com/prometheus/prometheus/pp/go/frames"
	"go.uber.org/zap"
)

const (
	defaultBufferSize = 1 << 10
)

func main() {
	if err := initLog(); err != nil {
		panic(fmt.Errorf("failed to initialize logger: %w", err))
	}

	cfg, err := makeConfig()
	if err != nil {
		zap.L().Fatal("failed init config", zap.Error(err))
	}

	// init storage
	storage, err := delivery.NewFileStorage(*cfg)
	if err != nil {
		zap.L().Fatal("failed init file storage", zap.Error(err))
	}

	ok, err := storage.FileExist()
	if err != nil {
		zap.L().Fatal("failed open file", zap.Error(err))
	}
	if !ok {
		zap.L().Fatal("file doesn't exist", zap.String("path", storage.GetPath()))
	}

	// open file
	if err = storage.OpenFile(); err != nil {
		zap.L().Fatal("failed open file", zap.Error(err))
	}

	size, err := storage.Size()
	if err != nil {
		zap.L().Fatal("failed get size", zap.Error(err))
	}

	reader := bufio.NewReaderSize(storage, defaultBufferSize)
	ctx := context.Background()
	if err = list(ctx, reader, int(size)); err != nil {
		zap.L().Fatal("failed read file", zap.Error(err))
	}
}

func initLog() error {
	config := zap.NewDevelopmentConfig()
	config.Level.SetLevel(zap.DebugLevel)

	logger, err := config.Build()
	if err != nil {
		return err
	}

	zap.ReplaceGlobals(logger)

	return nil
}

func makeConfig() (*delivery.FileStorageConfig, error) {
	//revive:disable:add-constant not need const
	if len(os.Args) < 2 {
		return nil, errors.New("path to refill is required as first argument")
	}
	path, err := filepath.Abs(os.Args[2])
	if err != nil {
		return nil, fmt.Errorf("fail to make absolute path by %s: %w", os.Args[1], err)
	}
	dir, file := filepath.Split(path)

	return &delivery.FileStorageConfig{
		Dir:      dir,
		FileName: strings.TrimSuffix(file, ".refill"),
	}, nil
}

var legend = map[frames.TypeFrame]string{
	frames.UnknownType:          "Unknown",
	frames.TitleType:            "Title",
	frames.DestinationNamesType: "DestinationNames",
	frames.SnapshotType:         "Snapshot",
	frames.SegmentType:          "Segment",
	frames.StatusType:           "Status",
	frames.RejectStatusType:     "Rejects",
	frames.RefillShardEOFType:   "RefillShardEOF",
}

// FileData - data to output.
type FileData struct {
	Type           string       `json:"type"`
	ContentVersion uint8        `json:"content_version"`
	Offset         int          `json:"offset"`
	Size           uint32       `json:"size"`
	SegmentInfo    *SegmentInfo `json:"segment_info,omitempty"`
}

// SegmentInfo - segment information.
type SegmentInfo struct {
	ShardID   uint16 `json:"shard_id"`
	SegmentID uint32 `json:"segment_id"`
}

func list(ctx context.Context, r *bufio.Reader, size int) error {
	je := json.NewEncoder(os.Stdout)
	var off int
	for {
		// read header frame
		h, err := frames.ReadHeader(ctx, r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		fd := FileData{
			Type:           legend[h.GetType()],
			ContentVersion: h.GetContentVersion(),
			Offset:         off,
			Size:           h.FrameSize(),
		}
		var n int
		if h.GetType() == frames.SegmentType {
			si := frames.NewSegmentInfoEmpty()
			n, err = si.ReadSegmentInfo(ctx, r, h)
			if err != nil {
				return err
			}

			fd.SegmentInfo = &SegmentInfo{
				ShardID:   si.GetShardID(),
				SegmentID: si.GetSegmentID(),
			}
		}

		if err = je.Encode(fd); err != nil {
			return err
		}

		// move cursor position
		move := int(h.GetSize() - uint32(n))
		off += int(h.FrameSize())
		if off > size {
			return errors.New("data oversize")
		}
		if _, err := r.Discard(move); err != nil {
			return err
		}
	}

	return nil
}
