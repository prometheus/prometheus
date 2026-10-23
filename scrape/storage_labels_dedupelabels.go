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

//go:build dedupelabels

package scrape

// reuseStorageLabels tells whether the scrape cache may keep the labels that
// the storage holds for a series instead of its own copy. With dedupelabels, the
// labels of a TSDB head series refer to the head's symbol table, which the head
// replaces after every truncation. Cache entries holding them would keep the
// old tables alive.
const reuseStorageLabels = false
