package remote

import (
	"sync"

	"github.com/prometheus/prometheus/tsdb/wlog"
)

type MarkerHandler interface {
	wlog.Marker
	UpdatePendingData(dataCount, dataSegment int)
	ProcessConsumedData(data map[int]int)
	Stop()
}

type markerHandler struct {
	markerFileHandler   MarkerFileHandler
	pendingDataMut      sync.Mutex
	latestDataSegment   int
	lastMarkedSegment   int
	pendingDataSegments map[int]int // Map of segment index to pending count
}

var (
	_ MarkerHandler = (*markerHandler)(nil)
)

func NewMarkerHandler(mfh MarkerFileHandler) MarkerHandler {
	mh := &markerHandler{
		latestDataSegment:   -1,
		lastMarkedSegment:   -1,
		pendingDataSegments: make(map[int]int),
		markerFileHandler:   mfh,
	}

	// Load the last marked segment from disk (if it exists).
	if lastSegment := mh.markerFileHandler.LastMarkedSegment(); lastSegment >= 0 {
		mh.lastMarkedSegment = lastSegment
	}

	return mh
}

func (mh *markerHandler) LastMarkedSegment() int {
	return mh.markerFileHandler.LastMarkedSegment()
}

// updatePendingData updates a counter for how much data is yet to be sent from each segment.
// "dataCount" will be added to the segment with ID "dataSegment".
func (mh *markerHandler) UpdatePendingData(dataCount, dataSegment int) {
	mh.pendingDataMut.Lock()
	defer mh.pendingDataMut.Unlock()

	mh.pendingDataSegments[dataSegment] += dataCount

	// We want to save segments whose data has been fully processed by the
	// QueueManager. To avoid too much overhead when appending data, we'll only
	// examine pending data per segment whenever a new segment is detected.
	//
	// This could cause our saved segment to lag behind multiple segments
	// depending on the rate that new segments are created and how long
	// data in other segments takes to be processed.
	if dataSegment <= mh.latestDataSegment {
		return
	}

	// We've received data for a new segment. We'll use this as a signal to see
	// if older segments have been fully processed.
	//
	// We want to mark the highest segment which has no more pending data and is
	// newer than our last mark.
	mh.latestDataSegment = dataSegment

	var markableSegment int
	for segment, count := range mh.pendingDataSegments {
		if segment >= dataSegment || count > 0 {
			continue
		}

		if segment > markableSegment {
			markableSegment = segment
		}

		// Clean up the pending map: the current segment has been completely
		// consumed and doesn't need to be considered for marking again.
		delete(mh.pendingDataSegments, segment)
	}

	if markableSegment > mh.lastMarkedSegment {
		mh.markerFileHandler.MarkSegment(markableSegment)
		mh.lastMarkedSegment = markableSegment
	}
}

// processConsumedData updates a counter for how many samples have been sent for each segment.
// "data" is a map of segment index to consumed data count (e.g. number of samples).
func (mh *markerHandler) ProcessConsumedData(data map[int]int) {
	mh.pendingDataMut.Lock()
	defer mh.pendingDataMut.Unlock()

	for segment, count := range data {
		if _, tracked := mh.pendingDataSegments[segment]; !tracked {
			//TODO: How is it possible to not track it? This should log an error?
			continue
		}
		mh.pendingDataSegments[segment] -= count
	}
}

func (mh *markerHandler) Stop() {
	mh.markerFileHandler.Stop()
}
