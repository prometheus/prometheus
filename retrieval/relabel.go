package retrieval

import (
	"regexp"
	"strings"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	pb "github.com/prometheus/prometheus/config/generated"
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
	pat, err := regexp.Compile(cfg.GetRegex())
	if err != nil {
		return nil, err
	}

	values := make([]string, 0, len(cfg.GetSourceLabel()))
	for _, name := range cfg.GetSourceLabel() {
		values = append(values, string(labels[clientmodel.LabelName(name)]))
	}
	val := strings.Join(values, cfg.GetSeparator())

	switch cfg.GetAction() {
	case pb.RelabelConfig_DROP:
		if pat.MatchString(val) {
			return nil, nil
		}
	case pb.RelabelConfig_KEEP:
		if !pat.MatchString(val) {
			return nil, nil
		}
	case pb.RelabelConfig_REPLACE:
		// If there is no match no replacement must take place.
		if !pat.MatchString(val) {
			break
		}
		res := pat.ReplaceAllString(val, cfg.GetReplacement())
		ln := clientmodel.LabelName(cfg.GetTargetLabel())
		if res == "" {
			delete(labels, ln)
		} else {
			labels[ln] = clientmodel.LabelValue(res)
		}
	default:
		panic("retrieval.relabel: unknown relabel action type")
	}
	return labels, nil
}
