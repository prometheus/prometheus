// types.go contains the Go<->C/C++ bridged types
// powered by C bindings.
// All dynamic memory in these types is handled by C/C++ bindings
// (Not the Go's GC!) so we must call corresponding C/C++ deallocation
// function to avoid memory leaks.
// Current types are: GoSegment, GoSliceByte, GoRedundant,
// GoSnapshot.
package internal

import (
	"io"
	"runtime"
	"unsafe"

	"github.com/gogo/protobuf/proto"
)

// GoSegment is GO wrapper for Segment. This wrapper is initialized from GO and is processed in C/C++ code.
// data - Go slice struct for cast to actual type in C/C++. Contains C/C++-managed memory
// buf - unsafe.Pointer for C++'s std::stringstream, is needed for retaining data from C++'s Redundant
// and must be deallocated using GoSegment's Destroy() function (to avoid memory leaks in C/C++ memory).
type GoSegment struct {
	data               CSlice
	buf                unsafe.Pointer
	samples            uint32
	series             uint32
	earliest           int64
	latest             int64
	remainingTableSize uint32
}

// NewGoSegment - init GoSegment.
func NewGoSegment() *GoSegment {
	gs := &GoSegment{
		data: CSlice{},
	}
	runtime.SetFinalizer(gs, func(gs *GoSegment) {
		CSegmentDestroy(unsafe.Pointer(gs))
	})
	return gs
}

// Size returns len of bytes
func (gs *GoSegment) Size() int64 {
	return int64(len(*(*[]byte)(unsafe.Pointer(&gs.data))))
}

// WriteTo implements io.WriterTo inerface
func (gs *GoSegment) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(*(*[]byte)(unsafe.Pointer(&gs.data)))
	runtime.KeepAlive(gs)
	return int64(n), err
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

func (gs *GoSegment) RemainingTableSize() uint32 {
	return gs.remainingTableSize
}

// GoDecodedSegment is the GO wrapper for decoded segment into remote write protobuf
type GoDecodedSegment struct {
	data          CSlice
	buf           unsafe.Pointer
	createdAtTSNS int64
	encodedAtTSNS int64
}

// NewGoDecodedSegment - init GoDecodedSegment.
func NewGoDecodedSegment() *GoDecodedSegment {
	ds := &GoDecodedSegment{
		data: CSlice{},
	}
	runtime.SetFinalizer(ds, func(ds *GoDecodedSegment) {
		CDecodedSegmentDestroy(unsafe.Pointer(ds))
	})
	return ds
}

// Size returns len of bytes
func (ds *GoDecodedSegment) Size() int64 {
	return int64(len(*(*[]byte)(unsafe.Pointer(&ds.data))))
}

// WriteTo implements io.WriterTo interface
func (ds *GoDecodedSegment) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(*(*[]byte)(unsafe.Pointer(&ds.data)))
	runtime.KeepAlive(ds)
	return int64(n), err
}

// CreatedAt returns timestamp in nanoseconds when source segment was created
func (ds *GoDecodedSegment) CreatedAt() int64 {
	return ds.createdAtTSNS
}

// EncodedAt returns timestamp in nanoseconds when source segment was encoded
func (ds *GoDecodedSegment) EncodedAt() int64 {
	return ds.encodedAtTSNS
}

// UnmarshalTo unmarshals data to given protobuf message
func (ds *GoDecodedSegment) UnmarshalTo(v proto.Unmarshaler) error {
	err := v.Unmarshal(*(*[]byte)(unsafe.Pointer(&ds.data)))
	runtime.KeepAlive(ds)
	return err
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
	gr := &GoRedundant{}
	runtime.SetFinalizer(gr, func(gr *GoRedundant) {
		CRedundantDestroy(unsafe.Pointer(gr))
	})
	return gr
}

// PointerData - get contained data, ONLY FOR TESTING PURPOSES.
func (gr *GoRedundant) PointerData() unsafe.Pointer {
	return gr.data
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
	gs := &GoSnapshot{
		data: CSlice{},
	}
	runtime.SetFinalizer(gs, func(gs *GoSnapshot) {
		CSnapshotDestroy(unsafe.Pointer(gs))
	})
	return gs
}

// Size returns count of bytes in snapshot
func (gs *GoSnapshot) Size() int64 {
	return int64(len(*(*[]byte)(unsafe.Pointer(&gs.data))))
}

// WriteTo implements io.WriterTo interface
func (gs *GoSnapshot) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(*(*[]byte)(unsafe.Pointer(&gs.data)))
	runtime.KeepAlive(gs)
	return int64(n), err
}

// GoErrorInfo is the Go-side wrapper for storing the error info from C/C++ APIs.
type GoErrorInfo struct {
	err CErrorInfo
}

// NewGoErrorInfo - init GoErrorInfo.
func NewGoErrorInfo() *GoErrorInfo {
	goerr := &GoErrorInfo{}
	runtime.SetFinalizer(goerr, func(goerr *GoErrorInfo) {
		CErrorInfoDestroy(goerr.err)
	})
	return goerr
}

// GetError - return error info from C++.
func (goerr *GoErrorInfo) GetError() error {
	if goerr.err == nil {
		return nil
	}
	err := CErrorInfoGetError(goerr.err)
	runtime.KeepAlive(goerr)
	return err
}

// GetStacktrace - return stacktrace info from C++.
func (goerr *GoErrorInfo) GetStacktrace() string {
	if goerr.err == nil {
		return ""
	}
	st := CErrorInfoGetStacktrace(goerr.err)
	runtime.KeepAlive(goerr)
	return st

}

// Destroy - destroy error C++.
func (goerr *GoErrorInfo) Destroy() {
	CErrorInfoDestroy(goerr.err)
}
