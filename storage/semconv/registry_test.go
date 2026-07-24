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

package semconv

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// embeddedRegistryFiles reads the embedded registry into a base-name → bytes
// map, the shape an operator-provided registry takes. Each call returns a fresh
// map so tests may mutate it.
func embeddedRegistryFiles(t *testing.T) map[string][]byte {
	t.Helper()
	entries, err := embeddedRegistry.ReadDir(registryPrefix)
	require.NoError(t, err)
	files := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := embeddedRegistry.ReadFile(registryPrefix + "/" + e.Name())
		require.NoError(t, err)
		files[e.Name()] = b
	}
	require.NotEmpty(t, files)
	return files
}

// tarGz builds a gzip-compressed tar of files (keyed by entry name) for tests.
func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, b := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(b))}))
		_, err := tw.Write(b)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestNewRegistrySource(t *testing.T) {
	files := embeddedRegistryFiles(t)
	e := newSchemaEngine(newRegistrySource(files))

	t.Run("resolves a registry/<name> path", func(t *testing.T) {
		b, err := e.readRegistryFile("registry/1.0.0")
		require.NoError(t, err)
		require.Equal(t, files["1.0.0"], b)
	})

	t.Run("resolves the schema index", func(t *testing.T) {
		b, err := e.readRegistryFile("registry/registry.yaml")
		require.NoError(t, err)
		require.NotEmpty(t, b)
	})

	t.Run("rejects unprefixed, traversal, and remote paths", func(t *testing.T) {
		for _, bad := range []string{"1.0.0", "registry/..", "registry/../etc/passwd", "/etc/passwd", "http://example.com/x"} {
			_, err := e.readRegistryFile(bad)
			require.Errorf(t, err, "expected %q to be rejected", bad)
		}
	})

	t.Run("reports a missing entry", func(t *testing.T) {
		_, err := e.readRegistryFile("registry/9.9.9")
		require.Error(t, err)
	})
}

func TestValidateRegistryFiles(t *testing.T) {
	t.Run("accepts the embedded layout", func(t *testing.T) {
		require.NoError(t, validateRegistryFiles(embeddedRegistryFiles(t)))
	})

	t.Run("rejects an empty registry", func(t *testing.T) {
		require.Error(t, validateRegistryFiles(nil))
	})

	t.Run("rejects an unparseable schema", func(t *testing.T) {
		require.Error(t, validateRegistryFiles(map[string][]byte{"registry.yaml": []byte("file_format: 9.9.9\n")}))
	})

	t.Run("rejects a registry with no schema", func(t *testing.T) {
		files := embeddedRegistryFiles(t)
		delete(files, "registry.yaml")
		require.Error(t, validateRegistryFiles(files))
	})

	t.Run("accepts a registry shipping a subset of referenced versions", func(t *testing.T) {
		// The schema references 1.0.0/1.1.0/1.2.0; shipping only some is valid —
		// operators provide the versions they query as anchors, not the whole history.
		files := embeddedRegistryFiles(t)
		delete(files, "1.2.0")
		require.NoError(t, validateRegistryFiles(files))
	})

	t.Run("rejects an unparseable semconv version file", func(t *testing.T) {
		files := embeddedRegistryFiles(t)
		files["1.0.0"] = []byte("groups: [unterminated")
		require.Error(t, validateRegistryFiles(files))
	})
}

func TestUntargz(t *testing.T) {
	t.Run("flattens entry paths to base names", func(t *testing.T) {
		want := map[string][]byte{
			"registry.yaml": []byte("file_format: 1.1.0\n"),
			"1.0.0":         []byte("groups: []\n"),
		}
		// Lay entries under a top-level directory to exercise flattening.
		nested := map[string][]byte{}
		for name, b := range want {
			nested["registry/"+name] = b
		}
		got, err := untargz(bytes.NewReader(tarGz(t, nested)))
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("rejects a duplicate base name", func(t *testing.T) {
		archive := tarGz(t, map[string][]byte{"a/1.0.0": []byte("x"), "b/1.0.0": []byte("y")})
		_, err := untargz(bytes.NewReader(archive))
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate")
	})

	t.Run("rejects too many entries", func(t *testing.T) {
		files := map[string][]byte{}
		for i := 0; i <= maxRegistryEntries; i++ {
			files[fmt.Sprintf("f%d", i)] = []byte("x")
		}
		_, err := untargz(bytes.NewReader(tarGz(t, files)))
		require.Error(t, err)
		require.Contains(t, err.Error(), "too many entries")
	})

	t.Run("rejects an oversized archive (decompression bomb)", func(t *testing.T) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		size := int64(maxRegistryDecompressedBytes) + (1 << 20)
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "big", Typeflag: tar.TypeReg, Mode: 0o644, Size: size}))
		chunk := make([]byte, 1<<20) // Zeros compress to almost nothing.
		for written := int64(0); written < size; {
			n := int64(len(chunk))
			if rem := size - written; rem < n {
				n = rem
			}
			_, err := tw.Write(chunk[:n])
			require.NoError(t, err)
			written += n
		}
		require.NoError(t, tw.Close())
		require.NoError(t, gz.Close())

		_, err := untargz(&buf)
		require.Error(t, err)
		require.Contains(t, err.Error(), "too large")
	})
}

func TestLoadRegistryFiles(t *testing.T) {
	t.Run("reads matched files keyed by base name", func(t *testing.T) {
		dir := t.TempDir()
		want := embeddedRegistryFiles(t)
		for name, b := range want {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), b, 0o644))
		}
		got, err := LoadRegistryFiles([]string{filepath.Join(dir, "*")})
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("errors when nothing matches", func(t *testing.T) {
		_, err := LoadRegistryFiles([]string{filepath.Join(t.TempDir(), "nope-*")})
		require.Error(t, err)
	})

	t.Run("rejects a duplicate base name across globs", func(t *testing.T) {
		dirA, dirB := t.TempDir(), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dirA, "1.0.0"), []byte("a"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dirB, "1.0.0"), []byte("b"), 0o644))
		_, err := LoadRegistryFiles([]string{filepath.Join(dirA, "*"), filepath.Join(dirB, "*")})
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate")
	})
}

func TestFetchRegistry(t *testing.T) {
	want := embeddedRegistryFiles(t)

	t.Run("downloads and unpacks an archive", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(tarGz(t, want))
		}))
		defer srv.Close()

		got, err := FetchRegistry(context.Background(), srv.Client(), srv.URL)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("errors on a non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := FetchRegistry(context.Background(), srv.Client(), srv.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected status")
	})
}

func TestBoundedReader(t *testing.T) {
	t.Run("passes through within the limit", func(t *testing.T) {
		br := &boundedReader{r: bytes.NewReader([]byte("hello")), what: "download", max: 5}
		b, err := io.ReadAll(br)
		require.NoError(t, err)
		require.Equal(t, "hello", string(b))
	})

	t.Run("fails past the limit naming the bound", func(t *testing.T) {
		br := &boundedReader{r: bytes.NewReader([]byte("hello world")), what: "download", max: 5}
		_, err := io.ReadAll(br)
		require.Error(t, err)
		require.Contains(t, err.Error(), "too large")
		require.Contains(t, err.Error(), "download")
	})
}
