package frames

import "fmt"

// TypeFrame - type of frame.
type TypeFrame uint8

// Validate - validate type frame.
func (tf TypeFrame) Validate() error {
	if tf < AuthType || tf > RefillShardEOFType {
		return fmt.Errorf("%w: %d", ErrUnknownFrameType, tf)
	}

	return nil
}

// Frame types
const (
	UnknownType TypeFrame = iota
	AuthType
	ResponseType
	RefillType
	TitleType
	DestinationNamesType
	SnapshotType
	SegmentType
	DrySegmentType
	StatusType
	RejectStatusType
	RefillShardEOFType
)
