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
	if len(parts) == 0 {
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

// metricGroupChange represents semconv metric group.
// NOTE(bwplotka): Only implementing fields that matter for querying.
type metricGroupChange struct {
	MetricName  string      `yaml:"metric_name"`
	Unit        string      `yaml:"unit"`
	ValuePromQL string      `yaml:"value_promql"`
	Attributes  []attribute `yaml:"attributes"`
}

func (m metricGroupChange) DirectUnit() string {
	return strings.TrimSuffix(strings.TrimPrefix(m.Unit, "{"), "}")
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
		return nil, err
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
		return nil, err
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
			return fmt.Errorf("http fetch %v: %w", url, err)
		}
		if resp.StatusCode/100 != 2 {
			// TODO(bwplotka): Print potential body?
			return fmt.Errorf("http fetch %v, got non-200 status: %v", url, resp.StatusCode)
		}

		// TODO(bwplotka): Add limit.
		b, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read all from http %v: %w", url, err)
		}
	} else {
		b, err = os.ReadFile(url)
		if err != nil {
			return fmt.Errorf("read all from file %v: %w", url, err)
		}
	}
	if err := yaml.Unmarshal(b, out); err != nil {
		return err
	}
	return nil
}
