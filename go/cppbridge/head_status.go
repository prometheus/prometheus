package cppbridge

import (
	"runtime"
)

type HeadStatus struct {
	TimeInterval struct {
		Min int64
		Max int64
	}
	LabelValueCountByLabelName []struct {
		Name  string
		Count uint32
	}
	SeriesCountByMetricName []struct {
		Name  string
		Count uint32
	}
	MemoryInBytesByLabelName []struct {
		Name string
		Size uint32
	}
	SeriesCountByLabelValuePair []struct {
		Name  string
		Value string
		Count uint32
	}
	NumSeries             uint32
	ChunkCount            uint32
	NumLabelPairs         uint32
	RuleQueriedSeries     uint32
	FederateQueriedSeries uint32
	OtherQueriedSeries    uint32
}

func GetHeadStatus(lss uintptr, dataStorage uintptr, limit int) *HeadStatus {
	status := &HeadStatus{}
	runtime.SetFinalizer(status, func(status *HeadStatus) {
		freeHeadStatus(status)
	})
	getHeadStatus(lss, dataStorage, status, limit)
	return status
}
