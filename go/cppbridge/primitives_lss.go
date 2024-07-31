package cppbridge

import (
	"runtime"
)

//
// LabelSetStorage
//

// CLSS interface for s-lss.
type CLSS interface {
	allocatedMemory() uint64
	pointer() uintptr
}

// LabelSetStorage go wrapper for C-LabelSetStorage.
type LabelSetStorage struct {
	clss       CLSS
	generation uint32
}

// NewLabelSetStorage init new LabelSetStorage.
func NewLabelSetStorage() (*LabelSetStorage, error) {
	clss, err := newLSS()
	if err != nil {
		return nil, err
	}

	return &LabelSetStorage{clss: clss}, nil
}

// NewOrderedLabelSetStorage init new LabelSetStorage with ordered lss.
func NewOrderedLabelSetStorage() (*LabelSetStorage, error) {
	clss, err := newOrderedLSS()
	if err != nil {
		return nil, err
	}

	return &LabelSetStorage{clss: clss}, nil
}

// AllocatedMemory return size of allocated memory for label sets in C++.
func (lss *LabelSetStorage) AllocatedMemory() uint64 {
	return lss.clss.allocatedMemory()
}

// Pointer return c-pointer.
func (lss *LabelSetStorage) Pointer() uintptr {
	return lss.clss.pointer()
}

// Reset init new c-lss.
func (lss *LabelSetStorage) Reset() error {
	var (
		clss CLSS
		err  error
	)
	switch lss.clss.(type) {
	case *labelSetStorage:
		clss, err = newLSS()
		if err != nil {
			return err
		}

	case *orderedLabelSetStorage:
		clss, err = newOrderedLSS()
		if err != nil {
			return err
		}
	}

	lss.clss = clss
	lss.generation++

	return nil
}

// Generation return generation lss.
func (lss *LabelSetStorage) Generation() uint32 {
	return lss.generation
}

//
// EncodingBimap
//

// labelSetStorage wrapper for C EncodingBimap.
type labelSetStorage struct {
	p uintptr
}

// newLSS init new LSS.
func newLSS() (*labelSetStorage, error) {
	p := primitivesLSSCtor()
	if p == 0 {
		return nil, ErrLSSNullPointer
	}

	lss := &labelSetStorage{p: p}
	runtime.SetFinalizer(lss, func(lss *labelSetStorage) {
		primitivesLSSDtor(lss.p)
	})
	return lss, nil
}

// allocatedMemory return size of allocated memory for label sets in C++.
func (lss *labelSetStorage) allocatedMemory() uint64 {
	if lss.p == 0 {
		return 0
	}
	return primitivesLSSAllocatedMemory(lss.p)
}

// pointer return c-pointer.
func (lss *labelSetStorage) pointer() uintptr {
	return lss.p
}

//
// OrderedEncodingBimap
//

// orderedLabelSetStorage wrapper for C OrderedEncodingBimap.
type orderedLabelSetStorage struct {
	p uintptr
}

// newOrderedLSS init new OrderedLSS.
func newOrderedLSS() (*orderedLabelSetStorage, error) {
	p := primitivesOrderedLSSCtor()
	if p == 0 {
		return nil, ErrLSSNullPointer
	}

	lss := &orderedLabelSetStorage{p: p}
	runtime.SetFinalizer(lss, func(lss *orderedLabelSetStorage) {
		primitivesOrderedLSSDtor(lss.p)
	})
	return lss, nil
}

// allocatedMemory - return size of allocated memory for label sets in C++.
func (lss *orderedLabelSetStorage) allocatedMemory() uint64 {
	if lss.p == 0 {
		return 0
	}
	return primitivesOrderedLSSAllocatedMemory(lss.p)
}

// pointer - return c-pointer.
func (lss *orderedLabelSetStorage) pointer() uintptr {
	return lss.p
}

// NilLabelSetStorage just for test.
type NilLabelSetStorage struct{}

func (*NilLabelSetStorage) allocatedMemory() uint64 {
	return 0
}

func (*NilLabelSetStorage) pointer() uintptr {
	return 0
}
