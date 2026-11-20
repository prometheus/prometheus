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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	// The first four publications exercise each command site, with more
	// publications left in the second variant to detect continued execution.
	plan := manifestPlans[1]
	for i := range 4 {
		t.Run(fmt.Sprintf("publication-%d", i+1), func(t *testing.T) {
			calls, output, err := runDocker(t, "common-docker-manifest", plan.variant, plan.tag, "amd64 arm64", i+1)
			require.Contains(t, output, "injected Docker failure", "Docker fault was not reached: %v", err)
			require.Error(t, err, "make returned success after Docker failed\n%s", output)
			require.Equal(t, manifestCalls(plan.tags[:i+1], "amd64 arm64"), calls, "unexpected publications after Docker failed")
		})
	}
}

func TestDockerManifestPlan(t *testing.T) {
	for _, plan := range manifestPlans {
		for _, archs := range []string{"amd64 arm64", ""} {
			t.Run(plan.name+"/archs="+archs, func(t *testing.T) {
				calls, output, err := runDocker(t, "common-docker-manifest", plan.variant, plan.tag, archs, 0)
				require.NoError(t, err, "make failed\n%s", output)
				require.Equal(t, manifestCalls(plan.tags, archs), calls)
			})
		}
	}
}

func TestDockerFailure(t *testing.T) {
	image := "example.test/prometheus-linux-amd64:"
	buildArgs := " --platform linux/amd64 --build-arg ARCH=amd64 --build-arg OS=linux --build-arg DISTROLESS_ARCH=amd64 ./"
	for _, plan := range []struct {
		target string
		tag    string
		calls  []string
	}{
		{"common-docker-amd64", "main", []string{
			"build -t " + image + "main -f Dockerfile" + buildArgs,
			"tag " + image + "main " + image + "main-busybox",
			"build -t " + image + "main-distroless -f Dockerfile.distroless" + buildArgs,
		}},
		{"common-docker-publish-amd64", "latest", []string{
			"push " + image + "latest-busybox",
			"push " + image + "latest",
			"push " + image + "v3-busybox",
			"push " + image + "v3",
		}},
		{"common-docker-tag-latest-amd64", "main", []string{
			"tag " + image + "main-busybox " + image + "latest-busybox",
			"tag " + image + "main-busybox " + image + "v3-busybox",
			"tag " + image + "main " + image + "latest",
			"tag " + image + "main " + image + "v3",
		}},
	} {
		for i := range plan.calls {
			t.Run(fmt.Sprintf("%s/command-%d", plan.target, i+1), func(t *testing.T) {
				calls, output, err := runDocker(t, plan.target, "busybox", plan.tag, "amd64", i+1)
				require.Contains(t, output, "injected Docker failure", "Docker fault was not reached: %v", err)
				require.Error(t, err, "make returned success after Docker failed\n%s", output)
				require.Equal(t, plan.calls[:i+1], calls, "unexpected Docker calls after failure")
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

func runDocker(t *testing.T, target, variant, tag, archs string, failAt int) ([]string, string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Docker publication recipes require a POSIX shell")
	}
	makefile, err := filepath.Abs("../Makefile.common")
	require.NoError(t, err)
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
    echo 'injected Docker failure' >&2
    exit 23
fi
`,
	} {
		if name == "Dockerfile" && variant != "" {
			contents += "LABEL io.prometheus.image.variant=\"" + variant + "\"\n"
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o700))
	}
	if target == "common-docker-amd64" {
		// Leave another build after the non-default variant's build.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile.extra"), []byte("FROM scratch\nLABEL io.prometheus.image.variant=\"extra\"\n"), 0o700))
	}
	cmd := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-f", makefile, target,
		"DOCKER_REPO=example.test", "DOCKER_IMAGE_NAME=prometheus", "DOCKER_IMAGE_TAG="+tag,
		"DOCKER_ARCHS="+archs)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOCKER_LOG="+filepath.Join(dir, "docker.log"), fmt.Sprintf("FAIL_AT=%d", failAt), "MAKEFLAGS=")
	output, runErr := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if runErr != nil {
		require.ErrorAs(t, runErr, &exitErr, "could not run make: %v", runErr)
	}
	log, err := os.ReadFile(filepath.Join(dir, "docker.log"))
	require.NoError(t, err)
	var calls []string
	if len(log) > 0 {
		calls = strings.Split(strings.TrimSuffix(string(log), "\n"), "\n")
	}
	return calls, string(output), runErr
}
