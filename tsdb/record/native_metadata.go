// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package record

import (
	"errors"
	"unsafe"
)

// MaxNativeMetadataVersions bounds the versions a native metadata history
// retains per series, and so the points of an override.
const MaxNativeMetadataVersions = 5

// ErrUnsupportedNativeMetadata is wrapped by the errors of decoding a native
// metadata record whose format, entry kind or flags this build doesn't define.
// Other decoding errors mean a record is malformed.
var ErrUnsupportedNativeMetadata = errors.New("unsupported native metadata record")

// NativeMetadataKind classifies a native metadata entry.
type NativeMetadataKind uint8

const (
	// NativeMetadataLegacy is an entry of a latest-value Metadata record. Its
	// one point has an unknown start, math.MinInt64.
	NativeMetadataLegacy NativeMetadataKind = iota
	// NativeMetadataGroup is one series' points from one transaction.
	NativeMetadataGroup
	// NativeMetadataOverride replaces a series' history and truncation flag.
	// Without points, it replaces the history with no versions.
	NativeMetadataOverride
	// NativeMetadataProvenance marks a checkpoint whose native metadata may be
	// trusted: one written with native metadata persisted, from inputs that
	// were trusted.
	NativeMetadataProvenance
	// NativeMetadataBoundary makes the native metadata that follows it in the
	// WAL trusted, after an untrusted span.
	NativeMetadataBoundary
)

// recordString returns b as a string, aliasing b if borrow is set. Empty
// strings never alias, so they cannot keep a record alive.
func recordString(b []byte, borrow bool) string {
	switch {
	case len(b) == 0:
		return ""
	case borrow:
		return unsafe.String(&b[0], len(b))
	default:
		return string(b)
	}
}
