package retrieval

import (
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
		// If there is no match no replacement must take place.
		if !cfg.Regex.MatchString(val) {
			break
		}
		res := cfg.Regex.ReplaceAllString(val, cfg.Replacement)
		if res == "" {
			delete(labels, cfg.TargetLabel)
		} else {
			labels[cfg.TargetLabel] = clientmodel.LabelValue(res)
		}
	default:
		panic(fmt.Errorf("retrieval.relabel: unknown relabel action type %q", cfg.Action))
	}
	return labels, nil
}
