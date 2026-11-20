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

// Package deprecation reports deprecated items seen in use.
package deprecation

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ExpectedRemoval is the release a deprecated item is expected to go in.
// NextMajorRelease only for items covered by the stability guarantees in
// docs/stability.md.
type ExpectedRemoval string

const (
	NextMajorRelease   ExpectedRemoval = "next_major_release"
	FutureMinorRelease ExpectedRemoval = "future_minor_release"
)

// scope says whether the user can make the notice go away.
type scope string

const (
	// scopeUser: fixing the usage and restarting clears the notice.
	scopeUser scope = "user"
	// scopeInternal: nothing the user does clears it, but they still have to
	// act before the item is removed.
	scopeInternal scope = "internal"
)

const docsURL = "https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices"

// logMsg must be the message of every deprecation log line: it is what makes
// them identifiable.
const logMsg = "Deprecated item in use, see details_url."

// Warn reports a deprecation next to its Mark call.
func Warn(l *slog.Logger, name string, args ...any) {
	l.Warn(logMsg, append([]any{"deprecation", name, "details_url", docsURL}, args...)...)
}

func Debug(l *slog.Logger, name string, args ...any) {
	l.Debug(logMsg, append([]any{"deprecation", name, "details_url", docsURL}, args...)...)
}

// Fprint is Warn for the few places that run before there is a logger.
func Fprint(w io.Writer, name string) {
	fmt.Fprintf(w, "time=%s level=WARN msg=%q deprecation=%q details_url=%s\n",
		time.Now().Format("2006-01-02T15:04:05.000Z07:00"), logMsg, name, docsURL)
}

var deprecationNotice = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "prometheus",
		Name:      "deprecation_notice",
		Help:      "Whether a deprecated feature, flag, configuration field, or behaviour has been seen in use since startup.",
		ConstLabels: prometheus.Labels{
			"details_url": docsURL,
		},
	},
	[]string{"name", "expected_removal_in", "scope"},
)

func Register(reg prometheus.Registerer) {
	reg.MustRegister(deprecationNotice)
}

// MarkDeprecated records a scope=user item, where the deprecated thing
// happens. name must match a heading in docs/stability.md. The notice is
// never withdrawn, only a restart clears it.
// Pair it with Warn or Debug.
func MarkDeprecated(name string, expectedRemovalIn ExpectedRemoval) {
	mark(name, expectedRemovalIn, scopeUser)
}

// MarkDeprecatedInternal is MarkDeprecated for things Prometheus does itself
// and cannot tell anyone depends on.
func MarkDeprecatedInternal(name string, expectedRemovalIn ExpectedRemoval) {
	mark(name, expectedRemovalIn, scopeInternal)
}

func mark(name string, expectedRemovalIn ExpectedRemoval, sc scope) {
	deprecationNotice.WithLabelValues(name, string(expectedRemovalIn), string(sc)).Set(1)
}
