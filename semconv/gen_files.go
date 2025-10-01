// Copyright 2025 The Prometheus Authors
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
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type metricID string

type semanticMetricID string

func (id metricID) semanticID() (_ semanticMetricID, revision int) {
	parts := strings.Split(string(id), ".")
	if len(parts) == 1 {
		return semanticMetricID(id), 0
	}

	var err error
	revision, err = strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return semanticMetricID(id), 0
	}

	// Number, assume revision.
	return semanticMetricID(strings.Join(parts[:len(parts)-1], ".")), revision
}

type changelog struct {
	Version int `yaml:"version"`

	MetricsChangelog map[semanticMetricID][]change `yaml:"metrics_changelog"`

	fetchTime time.Time
}

type change struct {
	Forward  metricGroupChange
	Backward metricGroupChange
}

// metricGroupChange represents a semconv metric group.
// NOTE(bwplotka): Only implementing fields that matter for querying.
type metricGroupChange struct {
	MetricName  string      `yaml:"metric_name"`
	Unit        string      `yaml:"unit"`
	ValuePromQL string      `yaml:"value_promql"`
	Attributes  []attribute `yaml:"attributes"`
}

func (m metricGroupChange) DirectUnit() string {
	if strings.HasPrefix(m.Unit, "{") {
		return strings.Trim(m.Unit, "{}") + "s"
	}
	// TODO(bwplotka): Yolo, fix it.
	return m.Unit
}

type attribute struct {
	Tag     string            `yaml:"tag"`
	Members []attributeMember `yaml:"members"`
}

type attributeMember struct {
	Value string `yaml:"value"`
}

func fetchChangelog(schemaChangelogURL string) (_ *changelog, err error) {
	ch := &changelog{}
	if err := fetchAndUnmarshal(schemaChangelogURL, ch); err != nil {
		return nil, fmt.Errorf("fetch changelog for __schema_url__=%q: %w", schemaChangelogURL, err)
	}
	return ch, nil
}

type ids struct {
	Version int `yaml:"version"`

	MetricsIDs               map[string][]versionedID `yaml:"metrics_ids"`
	uniqueNameToIdentity     map[string]string
	uniqueNameTypeToIdentity map[string]string

	fetchTime time.Time
}

func fetchIDs(schemaIDsURL string) (_ *ids, err error) {
	i := &ids{
		uniqueNameTypeToIdentity: make(map[string]string),
		uniqueNameToIdentity:     make(map[string]string),
	}
	if err := fetchAndUnmarshal(schemaIDsURL, i); err != nil {
		return nil, fmt.Errorf("fetch IDs for __schema_url__=%q: %w", schemaIDsURL, err)
	}

	for id := range i.MetricsIDs {
		var name, nameType string
		// Parse identity in a form of name~unit.type or name.type.
		parts := strings.Split(id, "~")
		if len(parts) > 1 {
			name = parts[0]

			unitAndType := strings.TrimPrefix(id, name+"~")
			parts = strings.Split(unitAndType, ".")
			nameType = name + "." + parts[len(parts)-1]
		} else {
			parts := strings.Split(id, ".")
			name = parts[0]
			nameType = id
		}

		if _, ok := i.uniqueNameToIdentity[name]; ok {
			// Not unique, put sentinel "" which will trigger error.
			i.uniqueNameToIdentity[name] = ""
		} else {
			i.uniqueNameToIdentity[name] = id
		}

		if _, ok := i.uniqueNameTypeToIdentity[nameType]; ok {
			// Not unique, put sentinel "" which will trigger error.
			i.uniqueNameTypeToIdentity[nameType] = ""
		} else {
			i.uniqueNameTypeToIdentity[nameType] = id
		}
	}
	return i, nil
}

type versionedID struct {
	ID           metricID `yaml:"id"`
	IntroVersion string   `yaml:"intro_version"`
}

func fetchAndUnmarshal[T any](url string, out *T) (err error) {
	var b []byte
	if strings.HasPrefix(url, "http") {
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("http fetch %s: %w", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			// TODO(bwplotka): Print potential body?
			return fmt.Errorf("http fetch %s, got non-200 status: %d", url, resp.StatusCode)
		}

		// TODO(bwplotka): Add limit.
		b, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read all from http %s: %w", url, err)
		}
	} else {
		b, err = os.ReadFile(url)
		if err != nil {
			return fmt.Errorf("read all from file %s: %w", url, err)
		}
	}
	if err := yaml.Unmarshal(b, out); err != nil {
		return err
	}
	return nil
}
