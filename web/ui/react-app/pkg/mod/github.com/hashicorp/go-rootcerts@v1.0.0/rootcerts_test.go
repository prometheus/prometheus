package rootcerts

import (
	"path/filepath"
	"testing"
)

const fixturesDir = "./test-fixtures"

func TestConfigureTLSHandlesNil(t *testing.T) {
	err := ConfigureTLS(nil, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestLoadCACertsHandlesNil(t *testing.T) {
	_, err := LoadCACerts(nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestLoadCACertsFromFile(t *testing.T) {
	path := testFixture("cafile", "cacert.pem")
	_, err := LoadCACerts(&Config{CAFile: path})
	if err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestLoadCACertsFromDir(t *testing.T) {
	path := testFixture("capath")
	_, err := LoadCACerts(&Config{CAPath: path})
	if err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestLoadCACertsFromDirWithSymlinks(t *testing.T) {
	path := testFixture("capath-with-symlinks")
	_, err := LoadCACerts(&Config{CAPath: path})
	if err != nil {
		t.Fatalf("err: %s", err)
	}
}

func testFixture(n ...string) string {
	parts := []string{fixturesDir}
	parts = append(parts, n...)
	return filepath.Join(parts...)
}
