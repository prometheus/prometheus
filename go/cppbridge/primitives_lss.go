package cppbridge

import (
	"runtime"
)

const (
	lssEncodingBimap uint32 = iota
	lssOrderedEncodingBimap
	lssQueryableEncodingBimap
)

//
// LabelSetStorage
//

// CLSS interface for s-lss.
type CLSS interface {
	allocatedMemory() uint64
	pointer() uintptr
}

type lssPointerWrapper struct {
	pointer uintptr
}

// LabelSetStorage go wrapper for C-LabelSetStorage.
type LabelSetStorage struct {
	pointer    *lssPointerWrapper
	lss_type   uint32
	generation uint32
}

// NewLabelSetStorage init new LabelSetStorage.
func newLabelSetStorage(lss_type uint32) *LabelSetStorage {
	return &LabelSetStorage{pointer: newLSS(lss_type), lss_type: lss_type}
}

// AllocatedMemory return size of allocated memory for label sets in C++.
func (lss *LabelSetStorage) AllocatedMemory() uint64 {
	return primitivesLSSAllocatedMemory(lss.pointer.pointer)
}

// Pointer return c-pointer.
func (lss *LabelSetStorage) Pointer() uintptr {
	return lss.pointer.pointer
}

// Reset init new c-lss.
func (lss *LabelSetStorage) Reset() {
	lss.pointer = newLSS(lss.lss_type)
	lss.generation++
}

// Generation return generation lss.
func (lss *LabelSetStorage) Generation() uint32 {
	return lss.generation
}

func NewLssStorage() *LabelSetStorage {
	return newLabelSetStorage(lssEncodingBimap)
}

func NewOrderedLssStorage() *LabelSetStorage {
	return newLabelSetStorage(lssOrderedEncodingBimap)
}

func NewQueryableLssStorage() *LabelSetStorage {
	return newLabelSetStorage(lssQueryableEncodingBimap)
}

// newLSS init new LSS.
func newLSS(lss_type uint32) *lssPointerWrapper {
	lss := &lssPointerWrapper{pointer: primitivesLSSCtor(lss_type)}
	runtime.SetFinalizer(lss, func(lss *lssPointerWrapper) {
		primitivesLSSDtor(lss.pointer)
	})
	return lss
}
