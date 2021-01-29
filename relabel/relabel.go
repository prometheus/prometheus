// Copyright 2015 The Prometheus Authors
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
	"fmt"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

// Process returns a relabeled copy of the given label set. The relabel configurations
// are applied in order of input.
// If a label set is dropped, nil is returned.
// May return the input labelSet modified.
func Process(labels model.LabelSet, cfgs ...*config.RelabelConfig) model.LabelSet {
	for _, cfg := range cfgs {
		labels = relabel(labels, cfg)
		if labels == nil {
			return nil
		}
	}
	return labels
}

func relabel(labels model.LabelSet, cfg *config.RelabelConfig) model.LabelSet {
	values := make([]string, 0, len(cfg.SourceLabels))
	for _, ln := range cfg.SourceLabels {
		values = append(values, string(labels[ln]))
	}
	val := strings.Join(values, cfg.Separator)

	switch cfg.Action {
	case config.RelabelDrop:
		if cfg.Regex.MatchString(val) {
			return nil
		}
	case config.RelabelKeep:
		if !cfg.Regex.MatchString(val) {
			return nil
		}
	case config.RelabelReplace:
		indexes := cfg.Regex.FindStringSubmatchIndex(val)
		// If there is no match no replacement must take place.
		if indexes == nil {
			break
		}
		target := model.LabelName(cfg.Regex.ExpandString([]byte{}, cfg.TargetLabel, val, indexes))
		if !target.IsValid() {
			delete(labels, model.LabelName(cfg.TargetLabel))
			break
		}
		res := cfg.Regex.ExpandString([]byte{}, cfg.Replacement, val, indexes)
		if len(res) == 0 {
			delete(labels, model.LabelName(cfg.TargetLabel))
			break
		}
		labels[target] = model.LabelValue(res)
	case config.RelabelHashMod:
		mod := sum64(md5.Sum([]byte(val))) % cfg.Modulus
		labels[model.LabelName(cfg.TargetLabel)] = model.LabelValue(fmt.Sprintf("%d", mod))
	case config.RelabelLabelMap:
		out := make(model.LabelSet, len(labels))
		// Take a copy to avoid infinite loops.
		for ln, lv := range labels {
			out[ln] = lv
		}
		for ln, lv := range labels {
			if cfg.Regex.MatchString(string(ln)) {
				res := cfg.Regex.ReplaceAllString(string(ln), cfg.Replacement)
				out[model.LabelName(res)] = lv
			}
		}
		labels = out
	case config.RelabelLabelDrop:
		for ln := range labels {
			if cfg.Regex.MatchString(string(ln)) {
				delete(labels, ln)
			}
		}
	case config.RelabelLabelKeep:
		for ln := range labels {
			if !cfg.Regex.MatchString(string(ln)) {
				delete(labels, ln)
			}
		}
	default:
		panic(fmt.Errorf("retrieval.relabel: unknown relabel action type %q", cfg.Action))
	}
	return labels
}

// sum64 sums the md5 hash to an uint64.
func sum64(hash [md5.Size]byte) uint64 {
	var s uint64

	for i, b := range hash {
		shift := uint64((md5.Size - i - 1) * 8)

		s |= uint64(b) << shift
	}
	return s
}
