package main

import (
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
	"github.com/prometheus/prometheus/pp/go/util"
	"go.uber.org/zap"
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
	storage, err := delivery.NewFileStorage(cfg)
	if err != nil {
		zap.L().Fatal("failed init file storage", zap.Error(err))
	}

	ok, err := storage.FileExist()
	if err != nil {
		zap.L().Fatal("failed open file", zap.Error(err))
	}
	if !ok {
		zap.L().Fatal("file doesn't exist")
	}

	// open file
	if err = storage.OpenFile(); err != nil {
		zap.L().Fatal("failed open file", zap.Error(err))
	}

	ctx := context.Background()
	if err = list(ctx, storage); err != nil {
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
	if len(os.Args) < 2 {
		return nil, errors.New("path to refill is required as first argument")
	}
	path, err := filepath.Abs(os.Args[1])
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
	Type      string `json:"type"`
	Offset    int64  `json:"offset"`
	Size      int32  `json:"size"`
	ShardID   uint16 `json:"shard_id,omitempty"`
	SegmentID uint32 `json:"segment_id,omitempty"`
}

func list(ctx context.Context, storage *delivery.FileStorage) error {
	je := json.NewEncoder(os.Stdout)
	var off int64
	for {
		// read header frame
		h, err := frames.ReadHeader(ctx, util.NewOffsetReader(storage, off))
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		_ = je.Encode(FileData{
			Type:      legend[h.GetType()],
			Offset:    off,
			Size:      h.FullSize(),
			ShardID:   h.GetShardID(),
			SegmentID: h.GetSegmentID(),
		})

		// move cursor position
		off += int64(h.SizeOf()) + int64(h.GetSize())
	}

	return nil
}
