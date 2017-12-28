package config

import (
	"net/url"
	"regexp"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/util/httputil"
)

type Reader interface {
	Read() []struct {

		// The job name to which the job label is set by default.
		JobName string
		// Indicator whether the scraped metrics should remain unmodified.
		HonorLabels bool
		// A set of query parameters with which the target is scraped.
		Params url.Values
		// How frequently to scrape the targets of this scrape config.
		ScrapeInterval time.Duration
		// The timeout for scraping targets of this config.
		ScrapeTimeout time.Duration
		// The HTTP resource path on which to fetch metrics from targets.
		MetricsPath string
		// The URL scheme with which to fetch metrics from targets.
		Scheme string
		// More than this many samples post metric-relabelling will cause the scrape to fail.
		SampleLimit uint

		HTTPClientConfig struct {
			// The HTTP basic authentication credentials for the targets.
			BasicAuth *struct {
				Username string
				Password string
			}
			// The bearer token for the targets.
			BearerToken string
			// The bearer token file for the targets.
			BearerTokenFile string
			// HTTP proxy server to use to connect to the targets.
			ProxyURL *url.URL
			// TLSConfig to use to connect to the targets.
			TLSConfig struct {
				// The CA cert to use for the targets.
				CAFile string
				// The client cert file for the targets.
				CertFile string
				// The client key file for the targets.
				KeyFile string
				// Used to verify the hostname for the targets.
				ServerName string
				// Disable target certificate validation.
				InsecureSkipVerify bool
			}
		}

		// List of target relabel configurations.
		RelabelConfigs []struct {
			// A list of labels from which values are taken and concatenated
			// with the configured separator in order.
			SourceLabels []string
			// Separator is the string between concatenated values from the source labels.
			Separator string
			// Regex against which the concatenation is matched.
			Regex struct {
				*regexp.Regexp
				original string
			}
			// Modulus to take of the hash of concatenated values from the source labels.
			Modulus uint64
			// TargetLabel is the label to which the resulting string is written in a replacement.
			// Regexp interpolation is allowed for the replace action.
			TargetLabel string
			// Replacement is the regex replacement pattern to be used.
			Replacement string
			// Action is the action to be performed for the relabeling.
			Action string
		}
		// List of metric relabel configurations.
		MetricRelabelConfigs []struct {
			// A list of labels from which values are taken and concatenated
			// with the configured separator in order.
			SourceLabels []string
			// Separator is the string between concatenated values from the source labels.
			Separator string
			// Regex against which the concatenation is matched.
			Regex struct {
				*regexp.Regexp
				original string
			}
			// Modulus to take of the hash of concatenated values from the source labels.
			Modulus uint64
			// TargetLabel is the label to which the resulting string is written in a replacement.
			// Regexp interpolation is allowed for the replace action.
			TargetLabel string
			// Replacement is the regex replacement pattern to be used.
			Replacement string
			// Action is the action to be performed for the relabeling.
			Action string
		}
	}
}

type Config struct {
	// The job name to which the job label is set by default.
	JobName string
	// Indicator whether the scraped metrics should remain unmodified.
	HonorLabels bool
	// A set of query parameters with which the target is scraped.
	Params url.Values
	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval time.Duration
	// The timeout for scraping targets of this config.
	ScrapeTimeout time.Duration
	// The HTTP resource path on which to fetch metrics from targets.
	MetricsPath string
	// The URL scheme with which to fetch metrics from targets.
	Scheme string
	// More than this many samples post metric-relabelling will cause the scrape to fail.
	SampleLimit uint

	HTTPClientConfig httputil.HTTPClientConfig

	// List of target relabel configurations.
	RelabelConfigs RelabelConfig
	// List of metric relabel configurations.
	MetricRelabelConfigs RelabelConfig
}

// RelabelConfig is the configuration for relabeling of target label sets.
type RelabelConfig []struct {
	// A list of labels from which values are taken and concatenated
	// with the configured separator in order.
	SourceLabels []string
	// Separator is the string between concatenated values from the source labels.
	Separator string
	// Regex against which the concatenation is matched.
	Regex struct {
		*regexp.Regexp
		original string
	}
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64
	// TargetLabel is the label to which the resulting string is written in a replacement.
	// Regexp interpolation is allowed for the replace action.
	TargetLabel string
	// Replacement is the regex replacement pattern to be used.
	Replacement string
	// Action is the action to be performed for the relabeling.
	Action string
}

const (
	// RelabelReplace performs a regex replacement.
	RelabelReplace = "replace"
	// RelabelKeep drops targets for which the input does not match the regex.
	RelabelKeep = "keep"
	// RelabelDrop drops targets for which the input does match the regex.
	RelabelDrop = "drop"
	// RelabelHashMod sets a label to the modulus of a hash of labels.
	RelabelHashMod = "hashmod"
	// RelabelLabelMap copies labels to other labelnames based on a regex.
	RelabelLabelMap = "labelmap"
	// RelabelLabelDrop drops any label matching the regex.
	RelabelLabelDrop = "labeldrop"
	// RelabelLabelKeep drops any label not matching the regex.
	RelabelLabelKeep = "labelkeep"
)

// // RelabelAction is the action to be performed on relabeling.
// type RelabelAction string

// Regexp encapsulates a regexp.Regexp and makes it YAML marshallable.
// type Regexp struct {
// 	*regexp.Regexp
// 	original string
// }

// TargetGroup is a set of targets with a common label set(production , test, staging etc.).
type TargetGroup struct {
	// Targets is a list of targets identified by a label set. Each target is
	// uniquely identifiable in the group by its address label.
	Targets []model.LabelSet
	// Labels is a set of labels that is common across all targets in the group.
	Labels model.LabelSet

	// Source is an identifier that describes a group of targets.
	Source string
}
