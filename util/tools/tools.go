// Copyright 2018 The Prometheus Authors
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

// Imports of tools we want "go mod vendor" to include.
// https://github.com/golang/go/issues/25624#issuecomment-395556484
// https://github.com/golang/go/issues/25922

// +build tools

package tools

import (
	_ "honnef.co/go/tools/cmd/staticcheck"
)
