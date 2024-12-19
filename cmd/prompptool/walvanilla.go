package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/wlog"

	"github.com/alecthomas/kingpin/v2"
	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type cmdWALVanillaToBlock struct {
	segmentSize   units.Base2Bytes
	compression   wlog.CompressionType
	blockDuration model.Duration
}

func registerCmdWALVanillaToBlock(cmd *cmdWALVanillaToBlock, clause *kingpin.CmdClause) {
	cmd.blockDuration = wlog.DefaultSegmentSize
	clause.Flag("storage.tsdb.wal-segment-size", "Size at which to split the tsdb WAL segment files. Example: 100MB").
		Hidden().PlaceHolder("<bytes").BytesVar(&cmd.segmentSize)
	clause.Flag("storage.tsdb.wal-compression-type", "Compression algorithm for the tsdb WAL.").
		Hidden().Default(string(wlog.CompressionSnappy)).EnumVar((*string)(&cmd.compression), string(wlog.CompressionSnappy), string(wlog.CompressionZstd))
	clause.Flag("storage.tsdb.min-block-duration", "Minimum duration of a data block before being persisted. For use in testing.").
		Hidden().Default("2h").SetValue(&cmd.blockDuration)
}

func (cmd *cmdWALVanillaToBlock) Do(ctx context.Context, workingDir string, logger log.Logger) error {
	walDir := filepath.Join(workingDir, "wal")
	_, err := os.Stat(walDir)
	if os.IsNotExist(err) {
		level.Debug(logger).Log("msg", "WAL is not found. Nothing to do")
		return nil
	}
	wal, err := wlog.NewSize(logger, nil, walDir, int(cmd.segmentSize), cmd.compression)
	if err != nil {
		return fmt.Errorf("read wlog: %w", err)
	}

	wblDir := filepath.Join(workingDir, wlog.WblDirName)
	wblSize, err := fileutil.DirSize(wblDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check wbl dir: %w", err)
	}
	var wbl *wlog.WL
	if wblSize > 0 {
		wbl, err = wlog.NewSize(logger, nil, wblDir, int(cmd.segmentSize), cmd.compression)
		if err != nil {
			return fmt.Errorf("read wbl wlog: %w", err)
		}
	}

	opts := tsdb.DefaultHeadOptions()
	opts.ChunkDirRoot = workingDir
	head, err := tsdb.NewHead(nil, logger, wal, wbl, opts, nil)
	if err != nil {
		return fmt.Errorf("create head: %w", err)
	}
	if err := head.Init(int64(math.MinInt64)); err != nil {
		return fmt.Errorf("init head: %w", err)
	}
	head.Index()

	blockDuration := int64(time.Duration(cmd.blockDuration) / time.Millisecond)
	compactor, err := tsdb.NewLeveledCompactorWithOptions(ctx, nil, logger, []int64{blockDuration}, nil, tsdb.LeveledCompactorOptions{})
	if err != nil {
		return fmt.Errorf("create compactor: %w", err)
	}

	mint := (head.MinTime() / blockDuration) * blockDuration
	maxt := mint + blockDuration
	for mint < head.MaxTime() {
		if err := ctx.Err(); err != nil {
			return err
		}
		rh := tsdb.NewRangeHead(head, mint, maxt)
		_, err := compactor.Write(workingDir, rh, mint, maxt, nil)
		if err != nil {
			return fmt.Errorf("write block: %w", err)
		}
		mint, maxt = maxt, maxt+blockDuration
	}
	if err := head.Close(); err != nil {
		return fmt.Errorf("close head: %w", err)
	}
	if wbl != nil {
		if err := os.Rename(wblDir, wblDir+".bak"); err != nil {
			level.Error(logger).Log("msg", "rename wbl", "error", err.Error())
		}
	}
	suffix := fmt.Sprintf(".%s.bak", time.Now().Format("2006010215040500"))
	return os.Rename(walDir, walDir+suffix)
}
