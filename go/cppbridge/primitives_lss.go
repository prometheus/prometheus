package cppbridge

import (
	"runtime"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/model/labels"
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

func (lss *LabelSetStorage) FindOrEmplace(labelSet model.LabelSet) uint32 {
	return primitivesLSSFindOrEmplace(lss.Pointer(), labelSet)
}

const (
	LSSQueryStatusNoPositiveMatchers uint32 = iota
	LSSQueryStatusRegexpError
	LSSQueryStatusNoMatch
	LSSQueryStatusMatch
)

type LSSQueryResult struct {
	status  uint32
	matches []uint32 // c allocated
}

func (r *LSSQueryResult) Status() uint32 {
	return r.status
}

func (r *LSSQueryResult) Matches() []uint32 {
	return r.matches
}

func (lss *LabelSetStorage) Query(matchers []model.LabelMatcher) *LSSQueryResult {
	result := &LSSQueryResult{}
	result.status, result.matches = primitivesLSSQuery(lss.Pointer(), matchers)
	runtime.SetFinalizer(result, func(result *LSSQueryResult) {
		freeBytes(*(*[]byte)(unsafe.Pointer(&result.matches)))
	})
	return result
}

type LabelSetStorageGetLabelSetsResult struct {
	labelSets []labels.Labels // c allocated
}

func (r *LabelSetStorageGetLabelSetsResult) LabelsSets() []labels.Labels {
	return r.labelSets
}

// GetLabelSets - returns copy of lss data.
func (lss *LabelSetStorage) GetLabelSets(labelSetIDs []uint32) *LabelSetStorageGetLabelSetsResult {
	result := &LabelSetStorageGetLabelSetsResult{labelSets: primitivesLSSGetLabelSets(lss.Pointer(), labelSetIDs)}
	runtime.SetFinalizer(result, func(result *LabelSetStorageGetLabelSetsResult) {
		primitivesLSSFreeLabelSets(result.labelSets)
	})
	return result
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
