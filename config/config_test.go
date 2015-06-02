package config

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v2"

	clientmodel "github.com/prometheus/client_golang/model"
)

var expectedConf = &Config{DefaultedConfig{
	GlobalConfig: &GlobalConfig{DefaultedGlobalConfig{
		ScrapeInterval:     Duration(15 * time.Second),
		ScrapeTimeout:      DefaultGlobalConfig.ScrapeTimeout,
		EvaluationInterval: Duration(30 * time.Second),

		Labels: clientmodel.LabelSet{
			"monitor": "codelab",
			"foo":     "bar",
		},
	}},

	RuleFiles: []string{
		"first.rules",
		"second.rules",
		"my/*.rules",
	},

	ScrapeConfigs: []*ScrapeConfig{
		{DefaultedScrapeConfig{
			JobName: "prometheus",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			TargetGroups: []*TargetGroup{
				{
					Targets: []clientmodel.LabelSet{
						{clientmodel.AddressLabel: "localhost:9090"},
						{clientmodel.AddressLabel: "localhost:9191"},
					},
					Labels: clientmodel.LabelSet{
						"my":   "label",
						"your": "label",
					},
				},
			},

			FileSDConfigs: []*FileSDConfig{
				{DefaultedFileSDConfig{
					Names:           []string{"foo/*.slow.json", "foo/*.slow.yml", "single/file.yml"},
					RefreshInterval: Duration(10 * time.Minute),
				}},
				{DefaultedFileSDConfig{
					Names:           []string{"bar/*.yaml"},
					RefreshInterval: Duration(30 * time.Second),
				}},
			},

			RelabelConfigs: []*RelabelConfig{
				{DefaultedRelabelConfig{
					SourceLabels: clientmodel.LabelNames{"job", "__meta_dns_srv_name"},
					TargetLabel:  "job",
					Separator:    ";",
					Regex:        &Regexp{*regexp.MustCompile("(.*)some-[regex]$")},
					Replacement:  "foo-${1}",
					Action:       RelabelReplace,
				}},
			},
		}},
		{DefaultedScrapeConfig{
			JobName: "service-x",

			ScrapeInterval: Duration(50 * time.Second),
			ScrapeTimeout:  Duration(5 * time.Second),

			BasicAuth: &BasicAuth{
				Username: "admin",
				Password: "password",
			},
			MetricsPath: "/my_path",
			Scheme:      "https",

			DNSSDConfigs: []*DNSSDConfig{
				{DefaultedDNSSDConfig{
					Names: []string{
						"first.dns.address.domain.com",
						"second.dns.address.domain.com",
					},
					RefreshInterval: Duration(15 * time.Second),
				}},
				{DefaultedDNSSDConfig{
					Names: []string{
						"first.dns.address.domain.com",
					},
					RefreshInterval: Duration(30 * time.Second),
				}},
			},

			RelabelConfigs: []*RelabelConfig{
				{DefaultedRelabelConfig{
					SourceLabels: clientmodel.LabelNames{"job"},
					Regex:        &Regexp{*regexp.MustCompile("(.*)some-[regex]$")},
					Separator:    ";",
					Action:       RelabelDrop,
				}},
			},
		}},
	},
}, ""}

func TestLoadConfig(t *testing.T) {
	c, err := LoadFromFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}
	bgot, err := yaml.Marshal(c)
	if err != nil {
		t.Errorf("%s", err)
	}
	bexp, err := yaml.Marshal(expectedConf)
	if err != nil {
		t.Errorf("%s", err)
	}
	expectedConf.original = c.original

	if !reflect.DeepEqual(c, expectedConf) {
		t.Errorf("%s: unexpected config result: \n\n%s\n expected\n\n%s", "testdata/conf.good.yml", bgot, bexp)
	}
}

var expectedErrors = []struct {
	filename string
	errMsg   string
}{
	{
		filename: "jobname.bad.yml",
		errMsg:   `"prom^etheus" is not a valid job name`,
	}, {
		filename: "jobname_dup.bad.yml",
		errMsg:   `found multiple scrape configs with job name "prometheus"`,
	}, {
		filename: "labelname.bad.yml",
		errMsg:   `"not$allowed" is not a valid label name`,
	}, {
		filename: "regex.bad.yml",
		errMsg:   "error parsing regexp",
	}, {
		filename: "rules.bad.yml",
		errMsg:   "invalid rule file path",
	},
}

func TestBadConfigs(t *testing.T) {
	for _, ee := range expectedErrors {
		_, err := LoadFromFile("testdata/" + ee.filename)
		if err == nil {
			t.Errorf("Expected error parsing %s but got none", ee.filename)
			continue
		}
		if !strings.Contains(err.Error(), ee.errMsg) {
			t.Errorf("Expected error for %s to contain %q but got: %s", ee.filename, ee.errMsg, err)
		}
	}
}
