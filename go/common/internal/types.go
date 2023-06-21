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
	data CSlice
	buf  unsafe.Pointer
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

// Destroy - Frees C/C++ allocated memory.
func (gs *GoSegment) Destroy() {
	CSliceWithStreamBufferDestroy(unsafe.Pointer(gs))
}

// GoSliceByte is the GO wrapper for Slice byte, init from GO and filling from C/C++.
// data - slice struct for cast in C/C++. Contained C/C++ memory
// buf - unsafe.Pointer for C++'s std::stringstream, is needed for retaining data from C++'s data
type GoSliceByte struct {
	data CSlice
	buf  unsafe.Pointer
}

// NewGoSliceByte - init GoSliceByte.
func NewGoSliceByte() *GoSliceByte {
	return &GoSliceByte{
		data: CSlice{},
	}
}

// Bytes - convert in go-slice byte from struct.
func (gs *GoSliceByte) Bytes() []byte {
	return *(*[]byte)((unsafe.Pointer(&gs.data)))
}

// Destroy - clear memory in C/C++.
func (gs *GoSliceByte) Destroy() {
	CSliceByteDestroy(unsafe.Pointer(gs))
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
	CSliceWithStreamBufferDestroy(unsafe.Pointer(gs))
}
