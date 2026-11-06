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

package scripts_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

var manifestPlans = []struct {
	name    string
	variant string
	tag     string
	tags    []string
}{
	{"named/main", "busybox", "main", []string{"main-busybox", "main", "main-distroless"}},
	{"named/latest", "busybox", "latest", []string{"latest-busybox", "latest", "v3-busybox", "v3", "latest-distroless", "v3-distroless"}},
	{"default/main", "", "main", []string{"main", "main-distroless"}},
	{"default/latest", "", "latest", []string{"latest", "v3", "latest-distroless", "v3-distroless"}},
	{"sanitized", "busybox", "v3.16.0+build.1", []string{"v3.16.0-build.1-busybox", "v3.16.0-build.1", "v3.16.0-build.1-distroless"}},
}

func TestDockerManifestFailure(t *testing.T) {
	for _, plan := range manifestPlans {
		for i := range plan.tags {
			t.Run(fmt.Sprintf("%s/publication-%d", plan.name, i+1), func(t *testing.T) {
				calls, output, err := runDockerManifest(t, plan.variant, plan.tag, "amd64 arm64", i+1)
				if !strings.Contains(output, "injected publication failure") {
					t.Fatalf("Docker fault was not reached: %v\n%s", err, output)
				}
				if err == nil {
					t.Errorf("make returned success after Docker failed\n%s", output)
				}
				if want := manifestCalls(plan.tags[:i+1], "amd64 arm64"); !slices.Equal(want, calls) {
					t.Errorf("unexpected publications after Docker failed:\nwant %q\ngot  %q", want, calls)
				}
			})
		}
	}
}

func TestDockerManifestPlan(t *testing.T) {
	for _, plan := range manifestPlans {
		for _, archs := range []string{"amd64 arm64", ""} {
			t.Run(plan.name+"/archs="+archs, func(t *testing.T) {
				calls, output, err := runDockerManifest(t, plan.variant, plan.tag, archs, 0)
				if err != nil {
					t.Fatalf("make failed: %v\n%s", err, output)
				}
				if want := manifestCalls(plan.tags, archs); !slices.Equal(want, calls) {
					t.Errorf("unexpected publications:\nwant %q\ngot  %q", want, calls)
				}
			})
		}
	}
}

func manifestCalls(tags []string, archs string) []string {
	var calls []string
	if archs == "" {
		return calls
	}
	for _, tag := range tags {
		call := "buildx imagetools create -t example.test/prometheus:" + tag
		for arch := range strings.FieldsSeq(archs) {
			call += " example.test/prometheus-linux-" + arch + ":" + tag
		}
		calls = append(calls, call)
	}
	return calls
}

func runDockerManifest(t *testing.T, variant, tag, archs string, failAt int) ([]string, string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Docker publication recipes require a POSIX shell")
	}
	makefile, err := filepath.Abs("../Makefile.common")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, contents := range map[string]string{
		"Dockerfile":            "FROM scratch\n",
		"Dockerfile.distroless": "FROM scratch\nLABEL io.prometheus.image.variant=\"distroless\"\n",
		"VERSION":               "3.16.0\n",
		"docker.log":            "",
		"docker": `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$DOCKER_LOG"
if [ "$(wc -l < "$DOCKER_LOG")" -eq "$FAIL_AT" ]; then
    echo 'injected publication failure' >&2
    exit 23
fi
`,
	} {
		if name == "Dockerfile" && variant != "" {
			contents += "LABEL io.prometheus.image.variant=\"" + variant + "\"\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("make", "--no-print-directory", "-f", makefile, "common-docker-manifest",
		"DOCKER_REPO=example.test", "DOCKER_IMAGE_NAME=prometheus", "DOCKER_IMAGE_TAG="+tag,
		"DOCKER_ARCHS="+archs)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOCKER_LOG="+filepath.Join(dir, "docker.log"), fmt.Sprintf("FAIL_AT=%d", failAt), "MAKEFLAGS=")
	output, runErr := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		t.Fatalf("could not run make: %v", runErr)
	}
	log, err := os.ReadFile(filepath.Join(dir, "docker.log"))
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	if len(log) > 0 {
		calls = strings.Split(strings.TrimSuffix(string(log), "\n"), "\n")
	}
	return calls, string(output), runErr
}
