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

package compliance

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	flag.Parse()

	skips := []string{receiverSkip, senderSkip}
	if existing := flag.Lookup("test.skip").Value.String(); existing != "" {
		skips = append([]string{existing}, skips...)
	}
	_ = flag.Set("test.skip", strings.Join(skips, "|"))

	os.Exit(m.Run())
}

func buildPrometheus(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "prometheus")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "../cmd/prometheus")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "failed to build prometheus binary: %s", string(out))
	return bin
}
