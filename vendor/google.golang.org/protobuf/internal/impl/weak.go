// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package impl

import (
	"reflect"

	pref "google.golang.org/protobuf/reflect/protoreflect"
)

// weakFields adds methods to the exported WeakFields type for internal use.
//
// The exported type is an alias to an unnamed type, so methods can't be
// defined directly on it.
type weakFields WeakFields

func (w *weakFields) get(num pref.FieldNumber) (_ pref.ProtoMessage, ok bool) {
	if *w == nil {
		return nil, false
	}
	m, ok := (*w)[int32(num)]
	if !ok {
		return nil, false
	}
	// As a legacy quirk, consider a typed nil to be unset.
	//
	// TODO: Consider fixing the generated set methods to clear the field
	// when provided with a typed nil.
	if v := reflect.ValueOf(m); v.Kind() == reflect.Ptr && v.IsNil() {
		return nil, false
	}
	return Export{}.ProtoMessageV2Of(m), true
}

func (w *weakFields) set(num pref.FieldNumber, m pref.ProtoMessage) {
	if *w == nil {
		*w = make(weakFields)
	}
	(*w)[int32(num)] = Export{}.ProtoMessageV1Of(m)
}

func (w *weakFields) clear(num pref.FieldNumber) {
	delete(*w, int32(num))
}
