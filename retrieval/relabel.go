package retrieval

import (
	"crypto/md5"
	"fmt"
	"strings"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
)

// Relabel returns a relabeled copy of the given label set. The relabel configurations
// are applied in order of input.
// If a label set is dropped, nil is returned.
func Relabel(labels clientmodel.LabelSet, cfgs ...*config.RelabelConfig) (clientmodel.LabelSet, error) {
	out := clientmodel.LabelSet{}
	for ln, lv := range labels {
		out[ln] = lv
	}
	var err error
	for _, cfg := range cfgs {
		if out, err = relabel(out, cfg); err != nil {
			return nil, err
		}
		if out == nil {
			return nil, nil
		}
	}
	return out, nil
}

func relabel(labels clientmodel.LabelSet, cfg *config.RelabelConfig) (clientmodel.LabelSet, error) {
	values := make([]string, 0, len(cfg.SourceLabels))
	for _, ln := range cfg.SourceLabels {
		values = append(values, string(labels[ln]))
	}
	val := strings.Join(values, cfg.Separator)

	switch cfg.Action {
	case config.RelabelDrop:
		if cfg.Regex.MatchString(val) {
			return nil, nil
		}
	case config.RelabelKeep:
		if !cfg.Regex.MatchString(val) {
			return nil, nil
		}
	case config.RelabelReplace:
		indexes := cfg.Regex.FindStringSubmatchIndex(val)
		// If there is no match no replacement must take place.
		if indexes == nil {
			break
		}
		res := cfg.Regex.ExpandString([]byte{}, cfg.Replacement, val, indexes)
		if len(res) == 0 {
			delete(labels, cfg.TargetLabel)
		} else {
			labels[cfg.TargetLabel] = clientmodel.LabelValue(res)
		}
	case config.RelabelHashMod:
		mod := sum64(md5.Sum([]byte(val))) % cfg.Modulus
		labels[cfg.TargetLabel] = clientmodel.LabelValue(fmt.Sprintf("%d", mod))
	case config.RelabelLabelMap:
		out := make(clientmodel.LabelSet, len(labels))
		// Take a copy to avoid infinite loops.
		for ln, lv := range labels {
			out[ln] = lv
		}
		for ln, lv := range labels {
			if cfg.Regex.MatchString(string(ln)) {
				res := cfg.Regex.ReplaceAllString(string(ln), cfg.Replacement)
				out[clientmodel.LabelName(res)] = lv
			}
		}
		labels = out
	default:
		panic(fmt.Errorf("retrieval.relabel: unknown relabel action type %q", cfg.Action))
	}
	return labels, nil
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
