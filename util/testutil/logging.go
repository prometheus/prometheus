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

package testutil

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

// RequireLogValue checks that value resolves to want (a string or nil) across
// logging styles, formats, and attribute placements.
func RequireLogValue(t *testing.T, key string, value, want any) {
	t.Helper()
	require.Equal(t, want, slog.AnyValue(value).Resolve().Any())

	for _, style := range []promslog.LogStyle{promslog.SlogStyle, promslog.GoKitStyle} {
		for _, formatName := range []string{"json", "logfmt"} {
			for _, placement := range []string{"direct", "with", "group", "with-group"} {
				t.Run(string(style)+"/"+formatName+"/"+placement, func(t *testing.T) {
					t.Helper()
					var output bytes.Buffer
					format := promslog.NewFormat()
					require.NoError(t, format.Set(formatName))
					logger := promslog.New(&promslog.Config{Writer: &output, Format: format, Style: style})
					switch placement {
					case "direct":
						logger.Info("test", key, value)
					case "with":
						logger.With(key, value).Info("test")
					case "group":
						logger.Info("test", slog.Group("group", key, value))
					case "with-group":
						logger.WithGroup("group").With(key, value).Info("test")
					}

					if formatName == "json" {
						var entry map[string]any
						require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
						if placement == "group" || placement == "with-group" {
							require.IsType(t, map[string]any{}, entry["group"])
							entry = entry["group"].(map[string]any)
						}
						require.Contains(t, entry, key)
						require.Equal(t, want, entry[key])
					} else {
						logKey := key
						if placement == "group" || placement == "with-group" {
							logKey = "group." + key
						}
						// Encode the expected value with slog's quoting rules.
						var expected bytes.Buffer
						slog.New(slog.NewTextHandler(&expected, &slog.HandlerOptions{
							ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
								if a.Key == logKey {
									return a
								}
								return slog.Attr{}
							},
						})).Info("", logKey, want)

						// The tested attribute is last in every placement.
						wantSuffix := " " + expected.String()
						require.True(t, strings.HasSuffix(output.String(), wantSuffix), "log output %q does not end with %q", output.String(), wantSuffix)
					}
				})
			}
		}
	}
}
