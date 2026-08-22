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
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// registryPrefix is the leading path segment every registry file is addressed
	// under (e.g. registry/1.0.0). It matches the embedded registry's directory and
	// the prefix encoded in registryURLRe.
	registryPrefix = "registry"

	// maxRegistryArchiveBytes caps the compressed download in FetchRegistry; the
	// decompressed output is bounded separately in untargz.
	maxRegistryArchiveBytes = 16 << 20

	// maxRegistryDecompressedBytes and maxRegistryEntries bound an archive after
	// decompression so a gzip/tar bomb cannot exhaust memory at startup. A registry
	// is a handful of small YAML files, so these are generous.
	maxRegistryDecompressedBytes = 16 << 20
	maxRegistryEntries           = 1024
)

// registryFiles is a registrySource backed by an in-memory map keyed by
// registry-root name (e.g. "registry.yaml", "1.0.0"). It is wrapped by
// newRegistrySource so callers address it under the registry/ prefix.
type registryFiles map[string][]byte

func (f registryFiles) ReadFile(name string) ([]byte, error) {
	b, ok := f[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
	}
	return b, nil
}

// prefixedRegistry adapts a registry-root source so that registry/<name> paths
// resolve to <name> in the inner source, matching how the embedded registry's
// registry/ directory is addressed.
type prefixedRegistry struct {
	prefix string
	inner  registrySource
}

func (p prefixedRegistry) ReadFile(name string) ([]byte, error) {
	rel, ok := strings.CutPrefix(name, p.prefix+"/")
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
	}
	return p.inner.ReadFile(rel)
}

// newRegistrySource wraps registry-root files (keyed by base name) as a
// registrySource addressable by registry/<name>, mirroring the embedded layout.
func newRegistrySource(files map[string][]byte) registrySource {
	return prefixedRegistry{prefix: registryPrefix, inner: registryFiles(files)}
}

// validateRegistryFiles reports whether files form a usable registry: a non-empty
// set in which every semver-named file (e.g. "1.0.0") parses as a semconv file,
// every other file (e.g. "registry.yaml") parses as an OTel schema, and at least
// one OTel schema is present. It rejects a malformed or schema-less registry at
// startup instead of letting it fail only at query time. It does not require a
// semconv file for every version a schema references — an OTel schema enumerates
// its full version history, but only versions queried as __semconv_url__ anchors
// need a file, which is the operator's responsibility to ship. It reuses
// loadSemconv/loadOTelSchema so validation matches what queries rely on.
func validateRegistryFiles(files map[string][]byte) error {
	if len(files) == 0 {
		return errors.New("registry is empty")
	}
	hasSchema := false
	for name, b := range files {
		if semverRe.MatchString(name) {
			if _, err := loadSemconv(b, name); err != nil {
				return fmt.Errorf("registry semconv %q: %w", name, err)
			}
			continue
		}
		if _, err := loadOTelSchema(b); err != nil {
			return fmt.Errorf("registry schema %q: %w", name, err)
		}
		hasSchema = true
	}
	if !hasSchema {
		return errors.New("registry has no OTel schema file")
	}
	return nil
}

// LoadRegistryFiles reads the registry-root files matched by the given globs,
// keyed by base name. It errors if no file matches or two matched files share a
// base name, which would otherwise silently collide in the registry namespace.
func LoadRegistryFiles(patterns []string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, pat := range patterns {
		matches, err := filepath.Glob(pat)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", pat, err)
		}
		for _, m := range matches {
			name := filepath.Base(m)
			if _, dup := files[name]; dup {
				return nil, fmt.Errorf("duplicate registry file %q (matched %q)", name, m)
			}
			b, err := os.ReadFile(m)
			if err != nil {
				return nil, fmt.Errorf("read %q: %w", m, err)
			}
			files[name] = b
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no registry files matched the configured paths")
	}
	return files, nil
}

// FetchRegistry downloads the .tar.gz registry archive at url using client and
// returns its files keyed by base name. The compressed download and the
// decompressed output are both size-bounded.
func FetchRegistry(ctx context.Context, client *http.Client, url string) (map[string][]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch registry %q: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch registry %q: unexpected status %s", url, resp.Status)
	}
	files, err := untargz(&boundedReader{r: resp.Body, what: "download", max: maxRegistryArchiveBytes})
	if err != nil {
		return nil, fmt.Errorf("unpack registry %q: %w", url, err)
	}
	return files, nil
}

// boundedReader fails with a clear error, naming the bound (what), once more than
// max bytes have been read from r. It guards both the compressed download and the
// decompressed output of a registry archive: a LimitReader would silently truncate,
// turning a gzip bomb into an opaque parse error instead.
type boundedReader struct {
	r    io.Reader
	what string
	n    int64
	max  int64
}

func (b *boundedReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.n += int64(n)
	if b.n > b.max {
		return n, fmt.Errorf("registry archive too large (%s exceeds %d bytes)", b.what, b.max)
	}
	return n, err
}

// untargz reads a gzip-compressed tar archive and returns its regular files keyed
// by base name, the layout AwareStorageWithRegistry expects for a registry. Entry
// paths are flattened to their base name, so a tarball laid out either flat or
// under a top-level directory both work; non-regular entries are skipped. The
// decompressed size and entry count are bounded to prevent a gzip/tar bomb, and a
// duplicate base name is rejected rather than silently overwritten.
func untargz(r io.Reader) (map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(&boundedReader{r: gz, what: "decompressed size", max: maxRegistryDecompressedBytes})
	files := map[string][]byte{}
	entries := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		entries++
		if entries > maxRegistryEntries {
			return nil, fmt.Errorf("registry archive has too many entries (> %d)", maxRegistryEntries)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Base(path.Clean(hdr.Name))
		if name == "." || name == ".." || name == "/" {
			continue
		}
		if _, dup := files[name]; dup {
			return nil, fmt.Errorf("duplicate registry file %q in archive", name)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", hdr.Name, err)
		}
		files[name] = b
	}
	if len(files) == 0 {
		return nil, errors.New("archive contained no files")
	}
	return files, nil
}
