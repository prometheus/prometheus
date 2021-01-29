// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Tamir Duberstein (tamird@gmail.com)

package protoutil

import (
	"fmt"
	"reflect"

	"github.com/cockroachdb/cockroach/pkg/util/syncutil"
	"github.com/gogo/protobuf/proto"
)

var verbotenKinds = [...]reflect.Kind{
	reflect.Array,
}

type typeKey struct {
	typ      reflect.Type
	verboten reflect.Kind
}

var types struct {
	syncutil.Mutex
	known map[typeKey]reflect.Type
}

func init() {
	types.known = make(map[typeKey]reflect.Type)
}

// Clone uses proto.Clone to return a deep copy of pb. It panics if pb
// recursively contains any instances of types which are known to be
// unsupported by proto.Clone.
//
// This function and its associated lint (see build/style_test.go) exist to
// ensure we do not attempt to proto.Clone types which are not supported by
// proto.Clone. This hackery is necessary because proto.Clone gives no direct
// indication that it has incompletely cloned a type; it merely logs to standard
// output (see
// https://github.com/golang/protobuf/blob/89238a3/proto/clone.go#L204).
//
// The concrete case against which this is currently guarding may be resolved
// upstream, see https://github.com/gogo/protobuf/issues/147.
func Clone(pb proto.Message) proto.Message {
	for _, verbotenKind := range verbotenKinds {
		if t := typeIsOrContainsVerboten(reflect.TypeOf(pb), verbotenKind); t != nil {
			panic(fmt.Sprintf("attempt to clone %T, which contains uncloneable field of type %s", pb, t))
		}
	}

	return proto.Clone(pb)
}

func typeIsOrContainsVerboten(t reflect.Type, verboten reflect.Kind) reflect.Type {
	types.Lock()
	defer types.Unlock()

	return typeIsOrContainsVerbotenLocked(t, verboten)
}

func typeIsOrContainsVerbotenLocked(t reflect.Type, verboten reflect.Kind) reflect.Type {
	key := typeKey{t, verboten}
	knownTypeIsOrContainsVerboten, ok := types.known[key]
	if !ok {
		knownTypeIsOrContainsVerboten = typeIsOrContainsVerbotenImpl(t, verboten)
		types.known[key] = knownTypeIsOrContainsVerboten
	}
	return knownTypeIsOrContainsVerboten
}

func typeIsOrContainsVerbotenImpl(t reflect.Type, verboten reflect.Kind) reflect.Type {
	switch t.Kind() {
	case verboten:
		return t

	case reflect.Map:
		if key := typeIsOrContainsVerbotenLocked(t.Key(), verboten); key != nil {
			return key
		}
		if value := typeIsOrContainsVerbotenLocked(t.Elem(), verboten); value != nil {
			return value
		}

	case reflect.Array, reflect.Ptr, reflect.Slice:
		if value := typeIsOrContainsVerbotenLocked(t.Elem(), verboten); value != nil {
			return value
		}

	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if field := typeIsOrContainsVerbotenLocked(t.Field(i).Type, verboten); field != nil {
				return field
			}
		}

	case reflect.Chan, reflect.Func:
		// Not strictly correct, but cloning these kinds is not allowed.
		return t

	}

	return nil
}
