package remote

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/tsdb/wlog"
)

type MarkerHandler interface {
	wlog.Marker

	UpdateReceivedData(segmentId, dataCount int) // Data queued for sending
	UpdateSentData(segmentId, dataCount int)     // Data which was sent or given up on sending

	Start()
	Stop()
}

type markerHandler struct {
	logger            log.Logger
	clientName        string
	markerFileHandler MarkerFileHandler
	lastMarkedSegment int
	dataIOUpdate      chan data
	quit              chan struct{}
}

// TODO: Rename this struct
type data struct {
	segmentId int
	dataCount int
}

func NewMarkerHandler(logger log.Logger, clientName string, mfh MarkerFileHandler) MarkerHandler {
	mh := &markerHandler{
		logger:            logger,
		clientName:        clientName,
		lastMarkedSegment: -1, // Segment ID last marked on disk.
		markerFileHandler: mfh,
		//TODO: What is a good size for the channel?
		dataIOUpdate: make(chan data, 100),
		quit:         make(chan struct{}),
	}

	return mh
}

func (mh *markerHandler) Start() {
	// Load the last marked segment from disk (if it exists).
	if lastSegment := mh.markerFileHandler.LastMarkedSegment(); lastSegment >= 0 {
		mh.lastMarkedSegment = lastSegment
	}
	level.Info(mh.logger).Log("msg", "Starting WAL marker handler", "queue", mh.clientName)
	go mh.updatePendingData()
}

func (mh *markerHandler) Stop() {
	// Firstly stop the Marker Handler, because it might want to use the Marker File Handler.
	mh.quit <- struct{}{}

	// Finally, stop the File Handler.
	mh.markerFileHandler.Stop()
}

func (mh *markerHandler) LastMarkedSegment() int {
	return mh.markerFileHandler.LastMarkedSegment()
}

func (mh *markerHandler) UpdateReceivedData(segmentId, dataCount int) {
	mh.dataIOUpdate <- data{
		segmentId: segmentId,
		dataCount: dataCount,
	}
}

func (mh *markerHandler) UpdateSentData(segmentId, dataCount int) {
	mh.dataIOUpdate <- data{
		segmentId: segmentId,
		dataCount: -1 * dataCount,
	}
}

// updatePendingData updates a counter for how much data is yet to be sent from each segment.
// "dataCount" will be added to the segment with ID "dataSegment".
func (mh *markerHandler) updatePendingData() {
	batchSegmentCount := make(map[int]int)

	for {
		select {
		case <-mh.quit:
			return
		case dataUpdate := <-mh.dataIOUpdate:
			//TODO: If count is less than 0, then log an error and remove the entry from the map?
			batchSegmentCount[dataUpdate.segmentId] += dataUpdate.dataCount
		}

		markableSegment := -1
		for segment, count := range batchSegmentCount {
			if count > 0 {
				continue
			}

			// TODO: should we just track the lowest segment ID with samples and the highest segment ID with samples?
			// then we can just check if the current segment was not the highest segment ID, ie is there any higher segment id with samples currently
			// in reality the % of the time that there will be samples for more than 1 segment is almost 0
			if segment > markableSegment && batchSegmentCount[segment+1] > 0 {
				markableSegment = segment
				// Clean up the pending map: the current segment has been completely
				// consumed and doesn't need to be considered for marking again.
				// we probably need to have a smarter cleanup either on a time ticker
				// or here we delete all keys that are a segment ID lower than us
				delete(batchSegmentCount, segment)
			}
		}

		if markableSegment > mh.lastMarkedSegment {
			// how to handle error here?
			if err := mh.markerFileHandler.MarkSegment(markableSegment); err == nil {
				mh.lastMarkedSegment = markableSegment
			}
		}
	}
}
