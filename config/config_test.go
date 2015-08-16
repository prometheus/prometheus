package config

import (
	"encoding/json"
	"io/ioutil"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v2"

	clientmodel "github.com/prometheus/client_golang/model"
)

var expectedConf = &Config{
	GlobalConfig: GlobalConfig{
		ScrapeInterval:     Duration(15 * time.Second),
		ScrapeTimeout:      DefaultGlobalConfig.ScrapeTimeout,
		EvaluationInterval: Duration(30 * time.Second),

		Labels: clientmodel.LabelSet{
			"monitor": "codelab",
			"foo":     "bar",
		},
	},

	RuleFiles: []string{
		"testdata/first.rules",
		"/absolute/second.rules",
		"testdata/my/*.rules",
	},

	ScrapeConfigs: []*ScrapeConfig{
		{
			JobName: "prometheus",

			HonorLabels:    true,
			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			BearerTokenFile: "testdata/valid_token_file",

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
				{
					Names:           []string{"foo/*.slow.json", "foo/*.slow.yml", "single/file.yml"},
					RefreshInterval: Duration(10 * time.Minute),
				},
				{
					Names:           []string{"bar/*.yaml"},
					RefreshInterval: Duration(5 * time.Minute),
				},
			},

			RelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: clientmodel.LabelNames{"job", "__meta_dns_srv_name"},
					TargetLabel:  "job",
					Separator:    ";",
					Regex:        &Regexp{*regexp.MustCompile("(.*)some-[regex]$")},
					Replacement:  "foo-${1}",
					Action:       RelabelReplace,
				},
			},
		},
		{
			JobName: "service-x",

			ScrapeInterval: Duration(50 * time.Second),
			ScrapeTimeout:  Duration(5 * time.Second),

			BasicAuth: &BasicAuth{
				Username: "admin_name",
				Password: "admin_password",
			},
			MetricsPath: "/my_path",
			Scheme:      "https",

			DNSSDConfigs: []*DNSSDConfig{
				{
					Names: []string{
						"first.dns.address.domain.com",
						"second.dns.address.domain.com",
					},
					RefreshInterval: Duration(15 * time.Second),
					Type:            "SRV",
				},
				{
					Names: []string{
						"first.dns.address.domain.com",
					},
					RefreshInterval: Duration(30 * time.Second),
					Type:            "SRV",
				},
			},

			RelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: clientmodel.LabelNames{"job"},
					Regex:        &Regexp{*regexp.MustCompile("(.*)some-[regex]$")},
					Separator:    ";",
					Action:       RelabelDrop,
				},
				{
					SourceLabels: clientmodel.LabelNames{"__address__"},
					TargetLabel:  "__tmp_hash",
					Modulus:      8,
					Separator:    ";",
					Action:       RelabelHashMod,
				},
				{
					SourceLabels: clientmodel.LabelNames{"__tmp_hash"},
					Regex:        &Regexp{*regexp.MustCompile("^1$")},
					Separator:    ";",
					Action:       RelabelKeep,
				},
			},
			MetricRelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: clientmodel.LabelNames{"__name__"},
					Regex:        &Regexp{*regexp.MustCompile("expensive_metric.*$")},
					Separator:    ";",
					Action:       RelabelDrop,
				},
			},
		},
		{
			JobName: "service-y",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ConsulSDConfigs: []*ConsulSDConfig{
				{
					Server:       "localhost:1234",
					Services:     []string{"nginx", "cache", "mysql"},
					TagSeparator: DefaultConsulSDConfig.TagSeparator,
					Scheme:       DefaultConsulSDConfig.Scheme,
				},
			},
		},
		{
			JobName: "service-z",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  Duration(10 * time.Second),

			MetricsPath: "/metrics",
			Scheme:      "http",

			ClientCert: &ClientCert{
				Cert: "testdata/valid_cert_file",
				Key:  "testdata/valid_key_file",
			},
			BearerToken: "avalidtoken",
		},
		{
			JobName: "service-kubernetes",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			KubernetesSDConfigs: []*KubernetesSDConfig{
				{
					Server:         "https://localhost:1234/",
					Username:       "myusername",
					Password:       "mypassword",
					KubeletPort:    10255,
					RequestTimeout: Duration(10 * time.Second),
					RetryInterval:  Duration(1 * time.Second),
				},
			},
		},
	},
	original: "",
}

func TestLoadConfig(t *testing.T) {
	// Parse a valid file that sets a global scrape timeout. This tests whether parsing
	// an overwritten default field in the global config permanently changes the default.
	if _, err := LoadFile("testdata/global_timeout.good.yml"); err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	bgot, err := yaml.Marshal(c)
	if err != nil {
		t.Fatalf("%s", err)
	}

	bexp, err := yaml.Marshal(expectedConf)
	if err != nil {
		t.Fatalf("%s", err)
	}
	expectedConf.original = c.original

	if !reflect.DeepEqual(c, expectedConf) {
		t.Fatalf("%s: unexpected config result: \n\n%s\n expected\n\n%s", "testdata/conf.good.yml", bgot, bexp)
	}

	// String method must not reveal authentication credentials.
	s := c.String()
	if strings.Contains(s, "admin_name") || strings.Contains(s, "admin_password") {
		t.Fatalf("config's String method reveals authentication credentials.")
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
		filename: "labelname2.bad.yml",
		errMsg:   `"not:allowed" is not a valid label name`,
	}, {
		filename: "regex.bad.yml",
		errMsg:   "error parsing regexp",
	}, {
		filename: "regex_missing.bad.yml",
		errMsg:   "relabel configuration requires a regular expression",
	}, {
		filename: "modulus_missing.bad.yml",
		errMsg:   "relabel configuration for hashmod requires non-zero modulus",
	}, {
		filename: "rules.bad.yml",
		errMsg:   "invalid rule file path",
	}, {
		filename: "unknown_attr.bad.yml",
		errMsg:   "unknown fields in scrape_config: consult_sd_configs",
	}, {
		filename: "bearertoken.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	}, {
		filename: "bearertoken_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, bearer_token & bearer_token_file must be configured",
	},
}

func TestBadConfigs(t *testing.T) {
	for _, ee := range expectedErrors {
		_, err := LoadFile("testdata/" + ee.filename)
		if err == nil {
			t.Errorf("Expected error parsing %s but got none", ee.filename)
			continue
		}
		if !strings.Contains(err.Error(), ee.errMsg) {
			t.Errorf("Expected error for %s to contain %q but got: %s", ee.filename, ee.errMsg, err)
		}
	}
}

func TestBadTargetGroup(t *testing.T) {
	content, err := ioutil.ReadFile("testdata/tgroup.bad.json")
	if err != nil {
		t.Fatal(err)
	}
	var tg TargetGroup
	err = json.Unmarshal(content, &tg)
	if err == nil {
		t.Errorf("Expected unmarshal error but got none.")
	}
}

func TestEmptyConfig(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Unexpected error parsing empty config file: %s", err)
	}
	exp := DefaultConfig

	if !reflect.DeepEqual(*c, exp) {
		t.Fatalf("want %v, got %v", exp, c)
	}
}

func TestEmptyGlobalBlock(t *testing.T) {
	c, err := Load("global:\n")
	if err != nil {
		t.Fatalf("Unexpected error parsing empty config file: %s", err)
	}
	exp := DefaultConfig
	exp.original = "global:\n"

	if !reflect.DeepEqual(*c, exp) {
		t.Fatalf("want %v, got %v", exp, c)
	}
}
