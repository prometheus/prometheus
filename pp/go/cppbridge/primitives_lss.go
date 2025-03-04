package cppbridge

import (
	"context"
	"runtime"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/model"
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

const (
	LSSQuerySourceRule uint32 = iota
	LSSQuerySourceFederate
	LSSQuerySourceOther
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

func (lss *LabelSetStorage) Query(matchers []model.LabelMatcher, querySource uint32) *LSSQueryResult {
	result := &LSSQueryResult{}
	result.status, result.matches = primitivesLSSQuery(lss.Pointer(), matchers, querySource)
	runtime.SetFinalizer(result, func(result *LSSQueryResult) {
		freeBytes(*(*[]byte)(unsafe.Pointer(&result.matches)))
	})
	return result
}

type LSSQueryLabelNamesResult struct {
	status uint32
	names  []string // c allocated
}

func (r *LSSQueryLabelNamesResult) Status() uint32 {
	return r.status
}

func (r *LSSQueryLabelNamesResult) Names() []string {
	return r.names
}

func (lss *LabelSetStorage) QueryLabelNames(matchers []model.LabelMatcher) *LSSQueryLabelNamesResult {
	result := &LSSQueryLabelNamesResult{}
	result.status, result.names = primitivesLSSQueryLabelNames(lss.Pointer(), matchers)
	runtime.SetFinalizer(result, func(result *LSSQueryLabelNamesResult) {
		freeBytes(*(*[]byte)(unsafe.Pointer(&result.names)))
	})
	return result
}

type LSSQueryLabelValuesResult struct {
	status uint32
	values []string // c allocated
}

func (r *LSSQueryLabelValuesResult) Status() uint32 {
	return r.status
}

func (r *LSSQueryLabelValuesResult) Values() []string {
	return r.values
}

func (lss *LabelSetStorage) QueryLabelValues(label_name string, matchers []model.LabelMatcher) *LSSQueryLabelValuesResult {
	result := &LSSQueryLabelValuesResult{}
	result.status, result.values = primitivesLSSQueryLabelValues(lss.Pointer(), label_name, matchers)
	runtime.SetFinalizer(result, func(result *LSSQueryLabelValuesResult) {
		freeBytes(*(*[]byte)(unsafe.Pointer(&result.values)))
	})
	return result
}

type LabelSetStorageGetLabelSetsResult struct {
	labelSets []Labels // c allocated
}

func (r *LabelSetStorageGetLabelSetsResult) LabelsSets() []Labels {
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

type ctxCallerKey struct{}

// SetTraceID get from context callerID, if not exist return LSSQuerySourceOther.
func GetCaller(ctx context.Context) uint32 {
	v, ok := ctx.Value(ctxCallerKey{}).(uint32)
	if !ok {
		return LSSQuerySourceOther
	}

	if v >= LSSQuerySourceOther {
		return LSSQuerySourceOther
	}

	return v
}

// SetTraceID set callerID to context.
func SetCaller(parent context.Context, callerID uint32) context.Context {
	return context.WithValue(parent, ctxCallerKey{}, callerID)
}
