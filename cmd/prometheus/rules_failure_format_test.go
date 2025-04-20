package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/rules"
	"github.com/stretchr/testify/assert"
)

func TestRuleFileParsing(t *testing.T) {
	// Mock configuration
	cfgFile := struct {
		RuleFiles  []string
		configFile string
	}{
		RuleFiles:  []string{"testdata/*.rules"},
		configFile: "testdata/prometheus.yml",
	}

	// Create test data
	err := os.MkdirAll("testdata", 0755)
	assert.NoError(t, err)
	defer os.RemoveAll("testdata")

	// Create valid and invalid rule files
	validRuleFile := "testdata/valid.rules"
	invalidRuleFile := "testdata/invalid.rules"

	err = os.WriteFile(validRuleFile, []byte("groups:\n- name: test\n  rules:\n  - alert: TestAlert\n    expr: up == 0"), 0644)
	assert.NoError(t, err)

	err = os.WriteFile(invalidRuleFile, []byte("invalid content"), 0644)
	assert.NoError(t, err)

	// Test case: Valid rule file
	t.Run("ValidRuleFile", func(t *testing.T) {
		files, err := filepath.Glob(cfgFile.RuleFiles[0])
		assert.NoError(t, err)

		for _, fn := range files {
			_, err := rules.ParseFile(fn)
			assert.NoError(t, err, fmt.Sprintf("Failed to parse valid rule file: %s", fn))
		}
	})

	// Test case: Invalid rule file
	t.Run("InvalidRuleFile", func(t *testing.T) {
		files, err := filepath.Glob(cfgFile.RuleFiles[0])
		assert.NoError(t, err)

		for _, fn := range files {
			if fn == invalidRuleFile {
				_, err := rules.ParseFile(fn)
				assert.Error(t, err, fmt.Sprintf("Expected error for invalid rule file: %s", fn))
			}
		}
	})
}

// mockLogger is a simple mock implementation of a logger for testing purposes.
type mockLogger struct{}

func (m *mockLogger) Error(msg string, keysAndValues ...interface{}) {
	fmt.Printf("ERROR: %s - %v\n", msg, keysAndValues)
}
