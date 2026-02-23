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

package logging

import (
	"context"
	"log/slog"
)

// Use this instance of noopSlogHandler everywhere, to avoid duplicates and
// so that pointer-equality comparisions can be made for it.
var noopHandler = noopSlogHandler{}

/*
 * Noop slog.Handler. Surprisingly, slog doesn't provide this for itself.
 */
type noopSlogHandler struct{}

func (h noopSlogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}

func (h noopSlogHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (h noopSlogHandler) Close() error {
	return nil
}

func (h noopSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h noopSlogHandler) WithGroup(_ string) slog.Handler {
	return h
}
