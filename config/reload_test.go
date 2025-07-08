// Copyright 2024 The Prometheus Authors
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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateChecksum(t *testing.T) {
	tmpDir := t.TempDir()

	// Define paths for the temporary files.
	yamlFilePath := filepath.Join(tmpDir, "test.yml")
	ruleFile := "rule_file.yml"
	ruleFilePath := filepath.Join(tmpDir, ruleFile)
	scrapeConfigFile := "scrape_config.yml"
	scrapeConfigFilePath := filepath.Join(tmpDir, scrapeConfigFile)

	// Define initial and modified content for the files.
	originalRuleContent := "groups:\n- name: example\n  rules:\n  - alert: ExampleAlert"
	modifiedRuleContent := "groups:\n- name: example\n  rules:\n  - alert: ModifiedAlert"

	originalScrapeConfigContent := "scrape_configs:\n- job_name: example"
	modifiedScrapeConfigContent := "scrape_configs:\n- job_name: modified_example"

	testCases := []struct {
		name                 string
		ruleFilePath         string
		scrapeConfigFilePath string
	}{
		{
			name:                 "Auto reload using relative path.",
			ruleFilePath:         ruleFile,
			scrapeConfigFilePath: scrapeConfigFile,
		},
		{
			name:                 "Auto reload using absolute path.",
			ruleFilePath:         ruleFilePath,
			scrapeConfigFilePath: scrapeConfigFilePath,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Define YAML content referencing the rule and scrape config files.
			yamlContent := fmt.Sprintf(`
rule_files:
  - %s
scrape_config_files:
  - %s
`, tc.ruleFilePath, tc.scrapeConfigFilePath)

			// Write initial content to files.
			require.NoError(t, os.WriteFile(ruleFilePath, []byte(originalRuleContent), 0o644))
			require.NoError(t, os.WriteFile(scrapeConfigFilePath, []byte(originalScrapeConfigContent), 0o644))
			require.NoError(t, os.WriteFile(yamlFilePath, []byte(yamlContent), 0o644))

			// Generate the original checksum.
			originalChecksum := calculateChecksum(t, yamlFilePath)

			t.Run("Rule File Change", func(t *testing.T) {
				// Modify the rule file.
				require.NoError(t, os.WriteFile(ruleFilePath, []byte(modifiedRuleContent), 0o644))

				// Checksum should change.
				modifiedChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, modifiedChecksum)

				// Revert the rule file.
				require.NoError(t, os.WriteFile(ruleFilePath, []byte(originalRuleContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Scrape Config Change", func(t *testing.T) {
				// Modify the scrape config file.
				require.NoError(t, os.WriteFile(scrapeConfigFilePath, []byte(modifiedScrapeConfigContent), 0o644))

				// Checksum should change.
				modifiedChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, modifiedChecksum)

				// Revert the scrape config file.
				require.NoError(t, os.WriteFile(scrapeConfigFilePath, []byte(originalScrapeConfigContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Rule File Deletion", func(t *testing.T) {
				// Delete the rule file.
				require.NoError(t, os.Remove(ruleFilePath))

				// Checksum should change.
				deletedChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, deletedChecksum)

				// Restore the rule file.
				require.NoError(t, os.WriteFile(ruleFilePath, []byte(originalRuleContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Scrape Config Deletion", func(t *testing.T) {
				// Delete the scrape config file.
				require.NoError(t, os.Remove(scrapeConfigFilePath))

				// Checksum should change.
				deletedChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, deletedChecksum)

				// Restore the scrape config file.
				require.NoError(t, os.WriteFile(scrapeConfigFilePath, []byte(originalScrapeConfigContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Main File Change", func(t *testing.T) {
				// Modify the main YAML file.
				modifiedYamlContent := fmt.Sprintf(`
global:
  scrape_interval: 3s
rule_files:
  - %s
scrape_config_files:
  - %s
`, tc.ruleFilePath, tc.scrapeConfigFilePath)
				require.NoError(t, os.WriteFile(yamlFilePath, []byte(modifiedYamlContent), 0o644))

				// Checksum should change.
				modifiedChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, modifiedChecksum)

				// Revert the main YAML file.
				require.NoError(t, os.WriteFile(yamlFilePath, []byte(yamlContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Rule File Removed from YAML Config", func(t *testing.T) {
				// Modify the YAML content to remove the rule file.
				modifiedYamlContent := fmt.Sprintf(`
scrape_config_files:
  - %s
`, tc.scrapeConfigFilePath)
				require.NoError(t, os.WriteFile(yamlFilePath, []byte(modifiedYamlContent), 0o644))

				// Checksum should change.
				modifiedChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, modifiedChecksum)

				// Revert the YAML content.
				require.NoError(t, os.WriteFile(yamlFilePath, []byte(yamlContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Scrape Config Removed from YAML Config", func(t *testing.T) {
				// Modify the YAML content to remove the scrape config file.
				modifiedYamlContent := fmt.Sprintf(`
rule_files:
  - %s
`, tc.ruleFilePath)
				require.NoError(t, os.WriteFile(yamlFilePath, []byte(modifiedYamlContent), 0o644))

				// Checksum should change.
				modifiedChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, modifiedChecksum)

				// Revert the YAML content.
				require.NoError(t, os.WriteFile(yamlFilePath, []byte(yamlContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Empty Rule File", func(t *testing.T) {
				// Write an empty rule file.
				require.NoError(t, os.WriteFile(ruleFilePath, []byte(""), 0o644))

				// Checksum should change.
				emptyChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, emptyChecksum)

				// Restore the rule file.
				require.NoError(t, os.WriteFile(ruleFilePath, []byte(originalRuleContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})

			t.Run("Empty Scrape Config File", func(t *testing.T) {
				// Write an empty scrape config file.
				require.NoError(t, os.WriteFile(scrapeConfigFilePath, []byte(""), 0o644))

				// Checksum should change.
				emptyChecksum := calculateChecksum(t, yamlFilePath)
				require.NotEqual(t, originalChecksum, emptyChecksum)

				// Restore the scrape config file.
				require.NoError(t, os.WriteFile(scrapeConfigFilePath, []byte(originalScrapeConfigContent), 0o644))

				// Checksum should return to the original.
				revertedChecksum := calculateChecksum(t, yamlFilePath)
				require.Equal(t, originalChecksum, revertedChecksum)
			})
		})
	}
}

// calculateChecksum generates a checksum for the given YAML file path.
func calculateChecksum(t *testing.T, yamlFilePath string) string {
	checksum, err := GenerateChecksum(yamlFilePath)
	require.NoError(t, err)
	require.NotEmpty(t, checksum)
	return checksum
}
