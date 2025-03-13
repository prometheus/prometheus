package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
)

type cmdWALPPToBlock struct {
	blockDuration model.Duration
}

func registerCmdWALPPToBlock(cmd *cmdWALPPToBlock, clause *kingpin.CmdClause) {
	clause.Flag("storage.tsdb.min-block-duration", "Minimum duration of a data block before being persisted. For use in testing.").
		Default("2h").SetValue(&cmd.blockDuration)
}

func (cmd *cmdWALPPToBlock) Do(
	ctx context.Context,
	workingDir string,
	logger log.Logger,
	registerer prometheus.Registerer,
) error {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	var err error
	if workingDir, err = filepath.Abs(workingDir); err != nil {
		return err
	}

	level.Debug(logger).Log("msg", "read file log")
	fileLog, err := catalog.NewFileLogV2(filepath.Join(workingDir, "head.log"))
	if err != nil {
		return fmt.Errorf("failed init file log reader: %w", err)
	}

	level.Debug(logger).Log("msg", "read catalog log")
	clock := clockwork.NewRealClock()
	headCatalog, err := catalog.New(clock, fileLog, catalog.DefaultIDGenerator{})
	if err != nil {
		return fmt.Errorf("failed init head catalog: %w", err)
	}
	headRecords, err := headCatalog.List(
		func(record *catalog.Record) bool {
			return record.DeletedAt() == 0 &&
				(record.Status() == catalog.StatusNew || record.Status() == catalog.StatusActive || record.Status() == catalog.StatusRotated)
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		return fmt.Errorf("failed listed head catalog: %w", err)
	}
	level.Debug(logger).Log("msg", "catalog records", "len", len(headRecords))

	var inputRelabelerConfig []*config.InputRelabelerConfig
	bw := block.NewBlockWriter(workingDir, block.DefaultChunkSegmentSize, time.Duration(cmd.blockDuration), registerer)
	for _, headRecord := range headRecords {
		if err := ctx.Err(); err != nil {
			return err
		}
		level.Debug(logger).Log("msg", "load head", "id", headRecord.ID(), "dir", headRecord.Dir())
		h, _, _, err := head.Load(
			headRecord.ID(),
			0,
			filepath.Join(workingDir, headRecord.Dir()),
			inputRelabelerConfig,
			headRecord.NumberOfShards(),
			0,
			head.NoOpLastAppendedSegmentIDSetter{},
			registerer,
		)
		if err != nil {
			level.Error(logger).Log(
				"msg", "failed to load head",
				"id", headRecord.ID(),
				"dir", headRecord.Dir(),
				"err", err,
			)
		}

		if err = h.Finalize(); err != nil {
			level.Error(logger).Log(
				"msg", "failed to finalize head",
				"id", headRecord.ID(),
				"dir", headRecord.Dir(),
				"err", err,
			)
		}

		level.Debug(logger).Log("msg", "write block", "id", headRecord.ID(), "dir", headRecord.Dir())
		if err = h.ForEachShard(func(shard relabeler.Shard) error {
			return bw.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
		}); err != nil {
			return fmt.Errorf("failed to write tsdb block [id: %s, dir: %s]: %w", headRecord.ID(), headRecord.Dir(), err)
		}

		if _, setStatusErr := headCatalog.SetStatus(headRecord.ID(), catalog.StatusPersisted); setStatusErr != nil {
			return fmt.Errorf("failed to set catalog status Persisted: %w", setStatusErr)
		}

		if err = h.Close(); err != nil {
			level.Error(logger).Log(
				"msg", "failed close head",
				"id", headRecord.ID,
				"dir", headRecord.Dir,
				"err", err,
			)
		}
	}

	return fileLog.Close()
}
