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

package relabel

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/schema"
)

// metricHelpLabel is the synthetic label name used to read and mutate metric help metadata during relabeling.
const metricHelpLabel = "__help__"

var (
	// relabelTargetLegacy allows targeting labels with legacy Prometheus character set, plus ${<var>} variables for dynamic characters from source the metrics.
	relabelTargetLegacy = regexp.MustCompile(`^(?:(?:[a-zA-Z_]|\$(?:\{\w+\}|\w+))+\w*)+$`)

	DefaultRelabelConfig = Config{
		Action:      Replace,
		Separator:   ";",
		Regex:       MustNewRegexp("(.*)"),
		Replacement: "$1",
	}
)

// Action is the action to be performed on relabeling.
type Action string

const (
	// Replace performs a regex replacement.
	Replace Action = "replace"
	// Keep drops targets for which the input does not match the regex.
	Keep Action = "keep"
	// Drop drops targets for which the input does match the regex.
	Drop Action = "drop"
	// KeepEqual drops targets for which the input does not match the target.
	KeepEqual Action = "keepequal"
	// DropEqual drops targets for which the input does match the target.
	DropEqual Action = "dropequal"
	// HashMod sets a label to the modulus of a hash of labels.
	HashMod Action = "hashmod"
	// LabelMap copies labels to other labelnames based on a regex.
	LabelMap Action = "labelmap"
	// LabelDrop drops any label matching the regex.
	LabelDrop Action = "labeldrop"
	// LabelKeep drops any label not matching the regex.
	LabelKeep Action = "labelkeep"
	// Lowercase maps input letters to their lower case.
	Lowercase Action = "lowercase"
	// Uppercase maps input letters to their upper case.
	Uppercase Action = "uppercase"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *Action) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	switch act := Action(strings.ToLower(s)); act {
	case Replace, Keep, Drop, HashMod, LabelMap, LabelDrop, LabelKeep, Lowercase, Uppercase, KeepEqual, DropEqual:
		*a = act
		return nil
	}
	return fmt.Errorf("unknown relabel action %q", s)
}

// Config is the configuration for relabeling of target label sets.
type Config struct {
	// A list of labels from which values are taken and concatenated
	// with the configured separator in order.
	SourceLabels model.LabelNames `yaml:"source_labels,flow,omitempty" json:"source_labels,omitempty"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `yaml:"separator" json:"separator"`
	// Regex against which the concatenation is matched.
	Regex Regexp `yaml:"regex,omitempty" json:"regex,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `yaml:"modulus,omitempty" json:"modulus,omitempty"`
	// TargetLabel is the label to which the resulting string is written in a replacement.
	// Regexp interpolation is allowed for the replace action.
	TargetLabel string `yaml:"target_label,omitempty" json:"target_label,omitempty"`
	// Replacement is the regex replacement pattern to be used.
	Replacement string `yaml:"replacement" json:"replacement"`
	// Action is the action to be performed for the relabeling.
	Action Action `yaml:"action,omitempty" json:"action,omitempty"`
	// NameValidationScheme to use when validating labels.
	NameValidationScheme model.ValidationScheme `yaml:"-" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultRelabelConfig
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Regex.Regexp == nil {
		c.Regex = MustNewRegexp("")
	}
	return nil
}

func (c *Config) Validate(nameValidationScheme model.ValidationScheme) error {
	if c.Action == "" {
		return errors.New("relabel action cannot be empty")
	}
	if c.Modulus == 0 && c.Action == HashMod {
		return errors.New("relabel configuration for hashmod requires non-zero modulus")
	}
	if (c.Action == Replace || c.Action == HashMod || c.Action == Lowercase || c.Action == Uppercase || c.Action == KeepEqual || c.Action == DropEqual) && c.TargetLabel == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'target_label' value", c.Action)
	}

	// Relabel config validation scheme matches global if left blank.
	switch c.NameValidationScheme {
	case model.LegacyValidation, model.UTF8Validation:
	case model.UnsetValidation:
		c.NameValidationScheme = nameValidationScheme
	default:
		return fmt.Errorf("unknown relabel config name validation method specified, must be either '', 'legacy' or 'utf8', got %s", c.NameValidationScheme)
	}

	if c.Action == Replace && !varInRegexTemplate(c.TargetLabel) && !c.NameValidationScheme.IsValidLabelName(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}

	isValidLabelNameWithRegexVarFn := func(value string) bool {
		// UTF-8 allows ${} characters, so standard validation allow $variables by default.
		// TODO(bwplotka): Relabelling users cannot put $ and ${<...>} characters in metric names or values.
		// Design escaping mechanism to allow that, once valid use case appears.
		switch c.NameValidationScheme {
		case model.UTF8Validation:
			return c.NameValidationScheme.IsValidLabelName(value)
		default:
			// For legacy validation, use the legacy regex that allows $variables.
			return relabelTargetLegacy.MatchString(value)
		}
	}
	if c.Action == Replace && varInRegexTemplate(c.TargetLabel) && !isValidLabelNameWithRegexVarFn(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase || c.Action == Uppercase || c.Action == KeepEqual || c.Action == DropEqual) && !c.NameValidationScheme.IsValidLabelName(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase || c.Action == Uppercase || c.Action == KeepEqual || c.Action == DropEqual) && c.Replacement != DefaultRelabelConfig.Replacement {
		return fmt.Errorf("'replacement' can not be set for %s action", c.Action)
	}
	if c.Action == LabelMap && !isValidLabelNameWithRegexVarFn(c.Replacement) {
		return fmt.Errorf("%q is invalid 'replacement' for %s action", c.Replacement, c.Action)
	}
	if c.Action == HashMod && !c.NameValidationScheme.IsValidLabelName(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}

	if c.Action == DropEqual || c.Action == KeepEqual {
		if c.Regex != DefaultRelabelConfig.Regex ||
			c.Modulus != DefaultRelabelConfig.Modulus ||
			c.Separator != DefaultRelabelConfig.Separator ||
			c.Replacement != DefaultRelabelConfig.Replacement {
			return fmt.Errorf("%s action requires only 'source_labels' and `target_label`, and no other fields", c.Action)
		}
	}

	if c.Action == LabelDrop || c.Action == LabelKeep {
		if c.SourceLabels != nil ||
			c.TargetLabel != DefaultRelabelConfig.TargetLabel ||
			c.Modulus != DefaultRelabelConfig.Modulus ||
			c.Separator != DefaultRelabelConfig.Separator ||
			c.Replacement != DefaultRelabelConfig.Replacement {
			return fmt.Errorf("%s action requires only 'regex', and no other fields", c.Action)
		}
	}

	if c.Action == Replace && c.TargetLabel == model.MetricTypeLabel && !varInRegexTemplate(c.Replacement) {
		if !schema.IsValidMetricType(model.MetricType(c.Replacement)) && c.Replacement != "" && c.Replacement != string(model.MetricTypeUnknown) {
			return fmt.Errorf("%q is invalid 'replacement' for %s action on %s", c.Replacement, c.Action, model.MetricTypeLabel)
		}
	}

	return nil
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshalable.
type Regexp struct {
	*regexp.Regexp
}

// NewRegexp creates a new anchored Regexp and returns an error if the
// passed-in regular expression does not compile.
func NewRegexp(s string) (Regexp, error) {
	regex, err := regexp.Compile("^(?s:" + s + ")$")
	return Regexp{Regexp: regex}, err
}

// MustNewRegexp works like NewRegexp, but panics if the regular expression does not compile.
func MustNewRegexp(s string) Regexp {
	re, err := NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	r, err := NewRegexp(s)
	if err != nil {
		return err
	}
	*re = r
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (re Regexp) MarshalYAML() (any, error) {
	if re.Regexp == nil {
		return nil, nil
	}
	return re.String(), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (re *Regexp) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	r, err := NewRegexp(s)
	if err != nil {
		return err
	}
	*re = r
	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (re Regexp) MarshalJSON() ([]byte, error) {
	return json.Marshal(re.String())
}

// IsZero implements the yaml.IsZeroer interface.
func (re Regexp) IsZero() bool {
	return re.Regexp == DefaultRelabelConfig.Regex.Regexp
}

// String returns the original string used to compile the regular expression.
func (re Regexp) String() string {
	if re.Regexp == nil {
		return ""
	}

	str := re.Regexp.String()
	// Trim the anchor `^(?s:` prefix and `)$` suffix.
	return str[5 : len(str)-2]
}

// ProcessBuilder applies relabeling configurations (rules) to the labels in lb.
// The rules are applied in order of input. Returns false if the rule says to drop.
func ProcessBuilder(lb *labels.Builder, cfgs ...*Config) (keep bool) {
	for _, cfg := range cfgs {
		keep = relabel(cfg, lb, false)
		if !keep {
			return false
		}
	}
	return true
}

// usesMetadata returns true if any relabel configuration reads or writes
// metric metadata (__type__, __unit__, or __help__).
func usesMetadata(cfgs ...*Config) bool {
	for _, cfg := range cfgs {
		for _, sl := range cfg.SourceLabels {
			if isMetadataRelabelName(string(sl)) {
				return true
			}
		}
		if isMetadataRelabelName(cfg.TargetLabel) || varInRegexTemplate(cfg.TargetLabel) {
			return true
		}
	}
	return false
}

func isMetadataRelabelName(ln string) bool {
	return ln == model.MetricTypeLabel || ln == model.MetricUnitLabel || ln == metricHelpLabel
}

// setLabel sets name to val on lb, deleting __type__ when set to "unknown" during
// metadata relabeling so subsequent rules consistently read empty __type__ for unknown types.
func setLabel(lb *labels.Builder, name, val string, withMetadata bool) {
	if withMetadata && name == model.MetricTypeLabel && val == string(model.MetricTypeUnknown) {
		lb.Del(name)
		return
	}
	lb.Set(name, val)
}

func delMetadataLabels(lb *labels.Builder) {
	if lb.Get(model.MetricTypeLabel) != "" {
		lb.Del(model.MetricTypeLabel)
	}
	if lb.Get(model.MetricUnitLabel) != "" {
		lb.Del(model.MetricUnitLabel)
	}
	if lb.Get(metricHelpLabel) != "" {
		lb.Del(metricHelpLabel)
	}
}

// ProcessBuilderWithMetadata applies relabeling configurations (rules) to the labels in lb
// and metadata in meta. Any pre-existing __type__, __unit__, or __help__ labels in lb on entry
// are replaced by meta (or stripped when empty in meta), and removed from lb before returning.
// Bulk label actions (labeldrop, labelkeep, labelmap) only operate on regular series labels
// and leave metadata untouched.
func ProcessBuilderWithMetadata(lb *labels.Builder, meta metadata.Metadata, cfgs ...*Config) (keep bool, _ metadata.Metadata) {
	if !usesMetadata(cfgs...) {
		delMetadataLabels(lb)
		keep = ProcessBuilder(lb, cfgs...)
		if !keep {
			return false, metadata.Metadata{}
		}
		delMetadataLabels(lb)
		return true, meta
	}

	if meta.Type != "" && meta.Type != model.MetricTypeUnknown {
		lb.Set(model.MetricTypeLabel, string(meta.Type))
	} else if lb.Get(model.MetricTypeLabel) != "" {
		lb.Del(model.MetricTypeLabel)
	}
	if meta.Unit != "" {
		lb.Set(model.MetricUnitLabel, meta.Unit)
	} else if lb.Get(model.MetricUnitLabel) != "" {
		lb.Del(model.MetricUnitLabel)
	}
	if meta.Help != "" {
		lb.Set(metricHelpLabel, meta.Help)
	} else if lb.Get(metricHelpLabel) != "" {
		lb.Del(metricHelpLabel)
	}

	for _, cfg := range cfgs {
		if !relabel(cfg, lb, true) {
			return false, metadata.Metadata{}
		}
	}

	t := model.MetricType(lb.Get(model.MetricTypeLabel))
	switch {
	case schema.IsValidMetricType(t):
		meta.Type = t
	case t != "" || meta.Type != "":
		meta.Type = model.MetricTypeUnknown
	}
	meta.Unit = lb.Get(model.MetricUnitLabel)
	meta.Help = lb.Get(metricHelpLabel)

	delMetadataLabels(lb)
	return true, meta
}

func relabel(cfg *Config, lb *labels.Builder, withMetadata bool) (keep bool) {
	var va [16]string
	values := va[:0]
	if len(cfg.SourceLabels) > cap(values) {
		values = make([]string, 0, len(cfg.SourceLabels))
	}
	for _, ln := range cfg.SourceLabels {
		values = append(values, lb.Get(string(ln)))
	}
	val := strings.Join(values, cfg.Separator)

	switch cfg.Action {
	case Drop:
		if cfg.Regex.MatchString(val) {
			return false
		}
	case Keep:
		if !cfg.Regex.MatchString(val) {
			return false
		}
	case DropEqual:
		if lb.Get(cfg.TargetLabel) == val {
			return false
		}
	case KeepEqual:
		if lb.Get(cfg.TargetLabel) != val {
			return false
		}
	case Replace:
		// Fast path to add or delete label pair.
		if val == "" && cfg.Regex == DefaultRelabelConfig.Regex &&
			!varInRegexTemplate(cfg.TargetLabel) && !varInRegexTemplate(cfg.Replacement) {
			setLabel(lb, cfg.TargetLabel, cfg.Replacement, withMetadata)
			break
		}

		indexes := cfg.Regex.FindStringSubmatchIndex(val)
		// If there is no match no replacement must take place.
		if indexes == nil {
			break
		}
		target := string(cfg.Regex.ExpandString([]byte{}, cfg.TargetLabel, val, indexes))
		if !cfg.NameValidationScheme.IsValidLabelName(target) {
			break
		}
		res := cfg.Regex.ExpandString([]byte{}, cfg.Replacement, val, indexes)
		if len(res) == 0 || (withMetadata && target == model.MetricTypeLabel && string(res) == string(model.MetricTypeUnknown)) {
			lb.Del(target)
			break
		}
		lb.Set(target, string(res))
	case Lowercase:
		setLabel(lb, cfg.TargetLabel, strings.ToLower(val), withMetadata)
	case Uppercase:
		lb.Set(cfg.TargetLabel, strings.ToUpper(val))
	case HashMod:
		hash := md5.Sum([]byte(val))
		// Use only the last 8 bytes of the hash to give the same result as earlier versions of this code.
		mod := binary.BigEndian.Uint64(hash[8:]) % cfg.Modulus
		lb.Set(cfg.TargetLabel, strconv.FormatUint(mod, 10))
	case LabelMap:
		lb.Range(func(l labels.Label) {
			if withMetadata && isMetadataRelabelName(l.Name) {
				return
			}
			if cfg.Regex.MatchString(l.Name) {
				res := cfg.Regex.ReplaceAllString(l.Name, cfg.Replacement)
				setLabel(lb, res, l.Value, withMetadata)
			}
		})
	case LabelDrop:
		lb.Range(func(l labels.Label) {
			if withMetadata && isMetadataRelabelName(l.Name) {
				return
			}
			if cfg.Regex.MatchString(l.Name) {
				lb.Del(l.Name)
			}
		})
	case LabelKeep:
		lb.Range(func(l labels.Label) {
			if withMetadata && isMetadataRelabelName(l.Name) {
				return
			}
			if !cfg.Regex.MatchString(l.Name) {
				lb.Del(l.Name)
			}
		})
	default:
		panic(fmt.Errorf("relabel: unknown relabel action type %q", cfg.Action))
	}

	return true
}

func varInRegexTemplate(template string) bool {
	return strings.Contains(template, "$")
}
