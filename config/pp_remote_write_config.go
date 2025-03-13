// Copyright OpCore

package config

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/prometheus/common/config"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/model/relabel"
	pp_pkg_config "github.com/prometheus/prometheus/pp-pkg/config"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	relabelerconfig "github.com/prometheus/prometheus/pp/go/relabeler/config"
)

const (
	PrometheusProtocol = "prometheus"
	OpProtocol         = "odarix"
	ScrapePrefix       = "scrape_"
	// TransparentRelabeler relabeler without ralabeling.
	TransparentRelabeler = "transparent_relabeler"
)

var (
	// DefaultOpRemoteWriteConfig is the default remote write configuration.
	DefaultOpRemoteWriteConfig = OpRemoteWriteConfig{
		Protocol:          PrometheusProtocol,
		RemoteWriteConfig: DefaultRemoteWriteConfig,
	}
	emptyTLSConfig = config.TLSConfig{}
)

func (c *Config) GetReceiverConfig() (*pp_pkg_config.RemoteWriteReceiverConfig, error) {
	scfgs, err := c.GetScrapeConfigs()
	if err != nil {
		return nil, err
	}
	rcCfg := c.ReceiverConfig.Copy()

	for _, scfg := range scfgs {
		oprCfgs, err := convertingRelabelConfigs(scfg.MetricRelabelConfigs)
		if err != nil {
			return nil, err
		}

		rcCfg.Configs = append(
			rcCfg.Configs,
			&relabelerconfig.InputRelabelerConfig{Name: ScrapePrefix + scfg.JobName, RelabelConfigs: oprCfgs},
		)
	}

	rcCfg.Configs = append(
		rcCfg.Configs,
		&relabelerconfig.InputRelabelerConfig{Name: TransparentRelabeler},
	)

	return rcCfg, nil
}

func convertingRelabelConfigs(rCfgs []*relabel.Config) ([]*cppbridge.RelabelConfig, error) {
	var oprCfgs []*cppbridge.RelabelConfig
	raw, err := yaml.Marshal(rCfgs)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(raw, &oprCfgs); err != nil {
		return nil, err
	}

	return oprCfgs, nil
}

// RemoteWriteConfig is the configuration for writing to remote storage.
type OpRemoteWriteConfig struct {
	Protocol          string                 `yaml:"protocol,omitempty"`
	Destinations      []*OpDestinationConfig `yaml:"destinations"`
	RemoteWriteConfig `yaml:",inline"`
}

// SetDirectory joins any relative file paths with dir.
func (c *OpRemoteWriteConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)

	if len(c.Destinations) == 0 {
		return
	}

	for _, d := range c.Destinations {
		d.HTTPClientConfig.SetDirectory(dir)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpRemoteWriteConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultOpRemoteWriteConfig
	type plain OpRemoteWriteConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Validate validates OpRemoteWriteConfig, but also fills relevant default values from global config if needed.
func (c *OpRemoteWriteConfig) Validate() error {
	if len(c.Destinations) == 0 {
		// if Destinations == 0, validate RemoteWriteConfig and
		// create one Destinations from RemoteWriteConfig
		if err := c.RemoteWriteConfig.Validate(); err != nil {
			return err
		}
		name := c.RemoteWriteConfig.Name
		if name == "" {
			hash, err := toHash(c.RemoteWriteConfig)
			if err != nil {
				return err
			}
			name = hash[:6]
		}
		c.Destinations = []*OpDestinationConfig{
			{
				Name:             name,
				URL:              c.RemoteWriteConfig.URL,
				HTTPClientConfig: c.RemoteWriteConfig.HTTPClientConfig,
			},
		}
	}

	for _, d := range c.Destinations {
		switch c.Protocol {
		case OpProtocol:
			if err := d.OpValidate(); err != nil {
				return err
			}
		case PrometheusProtocol:
			if err := d.Validate(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown remote write protocol: %s", c.Protocol)
		}
	}

	return nil
}

// IsPrometheusProtocol check type protocol is prometheus.
func (c *OpRemoteWriteConfig) IsPrometheusProtocol() bool {
	return c.Protocol == PrometheusProtocol
}

// Validate validates RemoteWriteConfig, but also fills relevant default values from global config if needed.
func (c *RemoteWriteConfig) Validate() error {
	if c.URL == nil {
		return errors.New("url for remote_write is empty")
	}
	for _, rlcfg := range c.WriteRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null relabeling rule in remote write config")
		}
	}
	if err := validateHeaders(c.Headers); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	httpClientConfigAuthEnabled := c.HTTPClientConfig.BasicAuth != nil ||
		c.HTTPClientConfig.Authorization != nil || c.HTTPClientConfig.OAuth2 != nil

	if httpClientConfigAuthEnabled && (c.SigV4Config != nil || c.AzureADConfig != nil) {
		return fmt.Errorf("at most one of basic_auth, authorization, oauth2, sigv4, & azuread must be configured")
	}

	if c.SigV4Config != nil && c.AzureADConfig != nil {
		return fmt.Errorf("at most one of basic_auth, authorization, oauth2, sigv4, & azuread must be configured")
	}

	return nil
}

// OpDestinationConfig is the configuration for destination server.
type OpDestinationConfig struct {
	Name             string                  `yaml:"name"`
	URL              *config.URL             `yaml:"url"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpDestinationConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = OpDestinationConfig{}
	type plain OpDestinationConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return nil
}

// Validate validates OpDestinationConfig, but also fills relevant default values from global config if needed.
func (c *OpDestinationConfig) Validate() error {
	if c.Name == "" {
		return errors.New("destination name is empty")
	}

	if c.URL == nil {
		return errors.New("url for destination is empty")
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	return c.HTTPClientConfig.Validate()
}

// OpValidate validates OpDestinationConfig, but also fills relevant default values from global config if needed.
// Validate op protocol.
func (c *OpDestinationConfig) OpValidate() error {
	if err := c.Validate(); err != nil {
		return err
	}

	if c.HTTPClientConfig.TLSConfig == emptyTLSConfig {
		return errors.New("tls config for destination is empty")
	}

	return nil
}

// Used for hashing configs and diff'ing hashes in ApplyConfig.
func toHash(data interface{}) (string, error) {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(bytes)
	return hex.EncodeToString(hash[:]), nil
}
