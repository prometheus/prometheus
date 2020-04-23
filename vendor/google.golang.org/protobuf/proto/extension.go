// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proto

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// HasExtension reports whether an extension field is populated.
// It panics if ext does not extend m.
func HasExtension(m Message, ext protoreflect.ExtensionType) bool {
	return m.ProtoReflect().Has(ext.TypeDescriptor())
}

// ClearExtension clears an extension field such that subsequent
// HasExtension calls return false.
// It panics if ext does not extend m.
func ClearExtension(m Message, ext protoreflect.ExtensionType) {
	m.ProtoReflect().Clear(ext.TypeDescriptor())
}

// GetExtension retrieves the value for an extension field.
// If the field is unpopulated, it returns the default value for
// scalars and an immutable, empty value for lists, maps, or messages.
// It panics if ext does not extend m.
func GetExtension(m Message, ext protoreflect.ExtensionType) interface{} {
	return ext.InterfaceOf(m.ProtoReflect().Get(ext.TypeDescriptor()))
}

// SetExtension stores the value of an extension field.
// It panics if ext does not extend m or if value type is invalid for the field.
func SetExtension(m Message, ext protoreflect.ExtensionType, value interface{}) {
	m.ProtoReflect().Set(ext.TypeDescriptor(), ext.ValueOf(value))
}
