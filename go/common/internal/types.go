// types.go contains the Go<->C/C++ bridged types
// powered by C bindings.
// All dynamic memory in these types is handled by C/C++ bindings
// (Not the Go's GC!) so we must call corresponding C/C++ deallocation
// function to avoid memory leaks.
// Current types are: GoSegment, GoSliceByte, GoRedundant,
// GoSnapshot.
package internal

import (
	"unsafe"
)

// GoSegment is GO wrapper for Segment. This wrapper is initialized from GO and is processed in C/C++ code.
// data - Go slice struct for cast to actual type in C/C++. Contains C/C++-managed memory
// buf - unsafe.Pointer for C++'s std::stringstream, is needed for retaining data from C++'s Redundant
// and must be deallocated using GoSegment's Destroy() function (to avoid memory leaks in C/C++ memory).
type GoSegment struct {
	data     CSlice
	buf      unsafe.Pointer
	samples  uint32
	series   uint32
	earliest int64
	latest   int64
}

// NewGoSegment - init GoSegment.
func NewGoSegment() *GoSegment {
	return &GoSegment{
		data: CSlice{},
	}
}

// Bytes returns raw go-slice bytes from struct.
func (gs *GoSegment) Bytes() []byte {
	return *(*[]byte)((unsafe.Pointer(&gs.data)))
}

// Samples returns count of samples in segment
func (gs *GoSegment) Samples() uint32 {
	return gs.samples
}

// Series returns count of series in segment
func (gs *GoSegment) Series() uint32 {
	return gs.series
}

// Earliest returns timestamp in ms of earliest sample in segment
func (gs *GoSegment) Earliest() int64 {
	return gs.earliest
}

// Latest returns timestamp in ms of latest sample in segment
func (gs *GoSegment) Latest() int64 {
	return gs.latest
}

// Destroy - Frees C/C++ allocated memory.
func (gs *GoSegment) Destroy() {
	CSegmentDestroy(unsafe.Pointer(gs))
}

// GoSliceByte is the GO wrapper for decoded segment into remote write protobuf
type GoDecodedSegment struct {
	data          CSlice
	buf           unsafe.Pointer
	createdAtTSNS int64
	encodedAtTSNS int64
}

// NewGoDecodedSegment - init GoDecodedSegment.
func NewGoDecodedSegment() *GoDecodedSegment {
	return &GoDecodedSegment{
		data: CSlice{},
	}
}

// Bytes - convert in go-slice byte from struct.
func (ds *GoDecodedSegment) Bytes() []byte {
	return *(*[]byte)((unsafe.Pointer(&ds.data)))
}

// CreatedAt returns timestamp in nanoseconds when source segment was created
func (ds *GoDecodedSegment) CreatedAt() int64 {
	return ds.createdAtTSNS
}

// EncodedAt returns timestamp in nanoseconds when source segment was encoded
func (ds *GoDecodedSegment) EncodedAt() int64 {
	return ds.encodedAtTSNS
}

// Destroy - clear memory in C/C++.
func (ds *GoDecodedSegment) Destroy() {
	CDecodedSegmentDestroy(unsafe.Pointer(ds))
}

// GoRedundant is GO wrapper for Redundant. This wrapper is initialized from GO and is processed in C/C++ code.
// data - Go slice struct for cast to actual type in C/C++. Contains C/C++-managed memory
// buf - unsafe.Pointer for C++'s std::stringstream, is needed for retaining data from C++'s Redundant
// and must be deallocated using GoRedundant's Destroy() function (to avoid memory leaks in C/C++ memory).
type GoRedundant struct {
	data unsafe.Pointer
}

// NewGoRedundant - init GoRedundant.
func NewGoRedundant() *GoRedundant {
	return &GoRedundant{}
}

// PointerData - get contained data, ONLY FOR TESTING PURPOSES.
func (gr *GoRedundant) PointerData() unsafe.Pointer {
	return gr.data
}

// Destroy - clear memory in C/C++.
func (gr *GoRedundant) Destroy() {
	CRedundantDestroy(unsafe.Pointer(gr))
}

// GoSnapshot - GO wrapper for snapshot, init from GO and filling from C/C++.
// data - slice struct for cast in C/C++. Contains C/C++ memory
// buf - unsafe.Pointer for std::stringstream in C/C++, need for clear memory.
type GoSnapshot struct {
	data CSlice
	buf  unsafe.Pointer
}

// NewGoSnapshot - init GoSnapshot.
func NewGoSnapshot() *GoSnapshot {
	return &GoSnapshot{
		data: CSlice{},
	}
}

// Bytes - convert in go-slice byte from struct.
func (gs *GoSnapshot) Bytes() []byte {
	return *(*[]byte)((unsafe.Pointer(&gs.data)))
}

// Destroy - clear memory in C/C++.
func (gs *GoSnapshot) Destroy() {
	CSnapshotDestroy(unsafe.Pointer(gs))
}
