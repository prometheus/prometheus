package remote

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

type MarkerFileHandler interface {
	wlog.Marker
	MarkSegment(segment int)
	Stop()
}

type markerFileHandler struct {
	segmentToMark chan int
	quit          chan struct{}

	logger log.Logger

	lastMarkedSegmentFilePath string
}

var (
	_ MarkerFileHandler = (*markerFileHandler)(nil)
)

func NewMarkerFileHandler(logger log.Logger, walDir, markerId string) (MarkerFileHandler, error) {
	markerDir := filepath.Join(walDir, "remote", markerId)

	dir := filepath.Join(walDir, "remote", markerId)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, fmt.Errorf("error creating segment marker folder %q: %w", dir, err)
	}

	mfh := &markerFileHandler{
		segmentToMark:             make(chan int, 1),
		quit:                      make(chan struct{}),
		logger:                    logger,
		lastMarkedSegmentFilePath: filepath.Join(markerDir, "segment"),
	}

	//TODO: Should this be in a separate Start() function?
	go mfh.markSegmentAsync()

	return mfh, nil
}

// LastMarkedSegment implements wlog.Marker.
func (mfh *markerFileHandler) LastMarkedSegment() int {
	bb, err := ioutil.ReadFile(mfh.lastMarkedSegmentFilePath)
	if os.IsNotExist(err) {
		level.Warn(mfh.logger).Log("msg", "marker segment file does not exist", "file", mfh.lastMarkedSegmentFilePath)
		return -1
	} else if err != nil {
		level.Error(mfh.logger).Log("msg", "could not access segment marker file", "file", mfh.lastMarkedSegmentFilePath, "err", err)
		return -1
	}

	savedSegment, err := strconv.Atoi(string(bb))
	if err != nil {
		level.Error(mfh.logger).Log("msg", "could not read segment marker file", "file", mfh.lastMarkedSegmentFilePath, "err", err)
		return -1
	}

	if savedSegment < 0 {
		level.Error(mfh.logger).Log("msg", "invalid segment number inside marker file", "file", mfh.lastMarkedSegmentFilePath, "segment number", savedSegment)
		return -1
	}

	return savedSegment
}

// MarkSegment implements MarkerHandler.
func (mfh *markerFileHandler) MarkSegment(segment int) {
	var (
		segmentText = strconv.Itoa(segment)
		tmp         = mfh.lastMarkedSegmentFilePath + ".tmp"
	)

	if err := os.WriteFile(tmp, []byte(segmentText), 0o666); err != nil {
		level.Error(mfh.logger).Log("msg", "could not create segment marker file", "file", tmp, "err", err)
		return
	}
	if err := fileutil.Replace(tmp, mfh.lastMarkedSegmentFilePath); err != nil {
		level.Error(mfh.logger).Log("msg", "could not replace segment marker file", "file", mfh.lastMarkedSegmentFilePath, "err", err)
		return
	}

	level.Debug(mfh.logger).Log("msg", "updated segment marker file", "file", mfh.lastMarkedSegmentFilePath, "segment", segment)
}

// Stop implements MarkerHandler.
func (mfh *markerFileHandler) Stop() {
	level.Debug(mfh.logger).Log("msg", "waiting for marker file handler to shut down...")
	mfh.quit <- struct{}{}
}

func (mfh *markerFileHandler) markSegmentAsync() {
	for {
		select {
		case segmentToMark := <-mfh.segmentToMark:
			if segmentToMark >= 0 {
				var (
					segmentText = strconv.Itoa(segmentToMark)
					tmp         = mfh.lastMarkedSegmentFilePath + ".tmp"
				)

				if err := os.WriteFile(tmp, []byte(segmentText), 0o666); err != nil {
					level.Error(mfh.logger).Log("msg", "could not create segment marker file", "file", tmp, "err", err)
					return
				}
				if err := fileutil.Replace(tmp, mfh.lastMarkedSegmentFilePath); err != nil {
					level.Error(mfh.logger).Log("msg", "could not replace segment marker file", "file", mfh.lastMarkedSegmentFilePath, "err", err)
					return
				}

				level.Debug(mfh.logger).Log("msg", "updated segment marker file", "file", mfh.lastMarkedSegmentFilePath, "segment", segmentToMark)
			}
		case <-mfh.quit:
			level.Debug(mfh.logger).Log("msg", "quitting marker handler")
			return
		}
	}
}
