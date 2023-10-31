// types.go contains the Go<->C/C++ bridged types
// powered by C bindings.
// All dynamic memory in these types is handled by C/C++ bindings
// (Not the Go's GC!) so we must call corresponding C/C++ deallocation
// function to avoid memory leaks.
// Current types are: GoSegment, GoSliceByte, GoRedundant,
// GoSnapshot.
package internal

import (
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
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
	manyStateHandle    uint64
}

// NewGoSegment - init GoSegment.
func NewGoSegment() *GoSegment {
	gs := &GoSegment{
		data: CSlice{},
	}
	runtime.SetFinalizer(gs, func(gs *GoSegment) {
		CSegmentDestroy(unsafe.Pointer(gs)) //nolint:gosec // this is memory optimisation
	})
	return gs
}

// Size returns len of bytes
func (gs *GoSegment) Size() int64 {
	return int64(len(*(*[]byte)(unsafe.Pointer(&gs.data)))) //nolint:gosec // this is memory optimisation
}

// WriteTo implements io.WriterTo inerface
func (gs *GoSegment) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(*(*[]byte)(unsafe.Pointer(&gs.data))) //nolint:gosec // this is memory optimisation
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

// RemainingTableSize - remaining table size in encoders.
func (gs *GoSegment) RemainingTableSize() uint32 {
	return gs.remainingTableSize
}

// GoDecodedSegment is the GO wrapper for decoded segment into remote write protobuf
type GoDecodedSegment struct {
	data          CSlice
	buf           unsafe.Pointer
	createdAtTSNS int64
	encodedAtTSNS int64
	samples       uint32
	series        uint32
}

// NewGoDecodedSegment - init GoDecodedSegment.
func NewGoDecodedSegment() *GoDecodedSegment {
	ds := &GoDecodedSegment{
		data: CSlice{},
	}
	runtime.SetFinalizer(ds, func(ds *GoDecodedSegment) {
		CDecodedSegmentDestroy(unsafe.Pointer(ds)) //nolint:gosec // this is memory optimisation
	})
	return ds
}

// Size returns len of bytes
func (ds *GoDecodedSegment) Size() int64 {
	return int64(len(*(*[]byte)(unsafe.Pointer(&ds.data)))) //nolint:gosec // this is memory optimisation
}

// WriteTo implements io.WriterTo interface
func (ds *GoDecodedSegment) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(*(*[]byte)(unsafe.Pointer(&ds.data))) //nolint:gosec // this is memory optimisation
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

// Samples - returns number of samples when source segment was encoded.
func (ds *GoDecodedSegment) Samples() uint32 {
	return ds.samples
}

// Series - returns number of series when source segment was encoded.
func (ds *GoDecodedSegment) Series() uint32 {
	return ds.series
}

// UnmarshalTo unmarshals data to given protobuf message
func (ds *GoDecodedSegment) UnmarshalTo(v proto.Unmarshaler) error {
	err := v.Unmarshal(*(*[]byte)(unsafe.Pointer(&ds.data))) //nolint:gosec // this is memory optimisation
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
		CRedundantDestroy(unsafe.Pointer(gr)) //nolint:gosec // this is memory optimisation
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
		CSnapshotDestroy(unsafe.Pointer(gs)) //nolint:gosec // this is memory optimisation
	})
	return gs
}

// Size returns count of bytes in snapshot
func (gs *GoSnapshot) Size() int64 {
	return int64(len(*(*[]byte)(unsafe.Pointer(&gs.data)))) //nolint:gosec // this is memory optimisation
}

// WriteTo implements io.WriterTo interface
func (gs *GoSnapshot) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(*(*[]byte)(unsafe.Pointer(&gs.data))) //nolint:gosec // this is memory optimisation
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

type copiedErr struct {
	code uint64
	msg  string
	st   string
}

func (err *copiedErr) Error() string {
	return err.msg
}

func (err *copiedErr) Code() uint64 {
	return err.code
}

func (err *copiedErr) Stacktrace() string {
	return err.st
}

// Format implements fmt.Formatter interface
func (err *copiedErr) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%s\n\n%s", err.Error(), err.Stacktrace())
			return
		}
		fallthrough
	case 's':
		_, _ = io.WriteString(s, err.Error())
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", err.Error())
	}
}

// GetError - return error info from C++.
func (goerr *GoErrorInfo) GetError() error {
	if goerr.err == nil {
		return nil
	}
	msg := CErrorInfoGetError(goerr.err)
	st := CErrorInfoGetStacktrace(goerr.err)
	runtime.KeepAlive(goerr)
	return &copiedErr{
		code: getCodeFromMsg(msg),
		msg:  msg,
		st:   st,
	}
}

func getCodeFromMsg(msg string) uint64 {
	_, msgStartedWithCode, ok := strings.Cut(msg, "(): Exception ")
	if !ok {
		return 0
	}
	codeStr, _, _ := strings.Cut(msgStartedWithCode, ":")
	//revive:disable-next-line:add-constant this not constant
	code, _ := strconv.ParseUint(codeStr, 16, 64)
	return code
}

// GoRestoredResult - GO wrapper for result after restore decoder.
type GoRestoredResult struct {
	offset            uint64
	requiredSegmentID uint32
	restoredSegmentID uint32
}

// NewGoRestoredResult - init new GoRestoredResult.
func NewGoRestoredResult(requiredSegmentID uint32) *GoRestoredResult {
	grr := &GoRestoredResult{
		requiredSegmentID: requiredSegmentID,
	}
	return grr
}

// Offset - get restored offset.
func (grr *GoRestoredResult) Offset() uint64 {
	return grr.offset
}

// RequiredSegmentID - get required segmentID.
func (grr *GoRestoredResult) RequiredSegmentID() uint32 {
	return grr.requiredSegmentID
}

// RestoredSegmentID - get restored segmentID.
func (grr *GoRestoredResult) RestoredSegmentID() uint32 {
	return grr.restoredSegmentID
}

// GoMemInfoResult - go-type wrapper for c-type, result meminfo usage.
type GoMemInfoResult struct {
	inUse uint64
}

// NewGoMemstatResult - int new GoMemInfoResult.
func NewGoMemstatResult() *GoMemInfoResult {
	return &GoMemInfoResult{}
}

// InUse - return current c-memory in use.
func (gmr *GoMemInfoResult) InUse() uint64 {
	return gmr.inUse
}
