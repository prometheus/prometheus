package importers

import (
	"context"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	plabels "github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

// RuleImporter is the importer for rules
type RuleImporter struct {
	config RuleConfig
	groups map[string]*rules.Group

	apiClient v1.API

	headBlock *tsdb.Head
	// if we have an appender for each rule, then
	// we will create one big block for each new rule
	// are there any downsides to making very large blocks?
	// or 2 hr blocks?
	appenders map[string]storage.Appender
}

// RuleConfig is the config for the rule importer
type RuleConfig struct {
	Start        string
	End          string
	EvalInterval time.Duration
	URL          string
}

// NewRuleImporter creates a new rule importer
func NewRuleImporter(config RuleConfig) *RuleImporter {
	return &RuleImporter{
		config: config,
	}
}

// Init initializes the rule importer preparing a new block that will
// be used to store the blocks created from the rules
func (importer *RuleImporter) Init() error {
	chunkDir, _ := ioutil.TempDir("", "chunk_dir")
	head, err := tsdb.NewHead(nil, nil, nil, 1000, chunkDir, nil, tsdb.DefaultStripeSize, nil)
	if err != nil {
		return err
	}
	importer.headBlock = head
	config := api.Config{
		Address: importer.config.URL,
	}
	c, err := api.NewClient(config)
	if err != nil {
		return err
	}
	importer.apiClient = v1.NewAPI(c)
	return nil
}

// Close cleans up any open resources
func (importer *RuleImporter) Close() {
	importer.headBlock.Close()
	// todo: clean up tmpdirs
}

// Parse parses the groups and rules from a list of rules files
func (importer *RuleImporter) Parse(ctx context.Context, files []string) (errs []error) {
	groups := make(map[string]*rules.Group)

	for _, file := range files {
		ruleGroups, errs := rulefmt.ParseFile(file)
		if errs != nil {
			return errs
		}

		for _, ruleGroup := range ruleGroups.Groups {
			itv := importer.config.EvalInterval
			if ruleGroup.Interval != 0 {
				itv = time.Duration(ruleGroup.Interval)
			}

			rulez := make([]rules.Rule, 0, len(ruleGroup.Rules))
			for _, r := range ruleGroup.Rules {
				expr, err := parser.ParseExpr(r.Expr.Value)
				if err != nil {
					return []error{errors.Wrap(err, file)}
				}

				rulez = append(rulez, rules.NewRecordingRule(
					r.Record.Value,
					expr,
					labels.FromMap(r.Labels),
				))
			}

			groups[file+";"+ruleGroup.Name] = rules.NewGroup(rules.GroupOptions{
				Name:     ruleGroup.Name,
				File:     file,
				Interval: itv,
				Rules:    rulez,
			})
		}
	}

	importer.groups = groups
	return errs
}

// ImportAll evaluates all the groups and rules and creates new time series
// and stores in new blocks
func (importer *RuleImporter) ImportAll(ctx context.Context) error {
	for _, group := range importer.groups {
		for _, rule := range group.Rules() {
			// todo: what happens if there is an error importing one of the rules?
			importer.ImportRule(ctx, rule)
		}
	}
	return importer.CreateBlocks()
}

func (importer *RuleImporter) queryFn(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
	// what to do with warnings? maybe just print for now?
	val, _, err := importer.apiClient.Query(ctx, q, t)
	if err != nil {
		return promql.Vector{}, err
	}

	switch val.Type() {
	case model.ValVector:
		valVector := val.(model.Vector)
		return modelToPromqlVector(valVector), nil
	case model.ValScalar:
		valScalar := val.(*model.Scalar)
		return promql.Vector{promql.Sample{
			Metric: labels.Labels{},
			Point:  promql.Point{T: int64(valScalar.Timestamp), V: float64(valScalar.Value)},
		}}, nil
	// case model.ValMatrix/Instant:  todo: might need to add matrix and instant? not sure if those map to scalar/vector tho
	default:
		return nil, errors.New("rule result is wrong type")
	}
}

func modelToPromqlVector(modelValue model.Vector) promql.Vector {
	result := make(promql.Vector, 0, len(modelValue))

	for _, value := range modelValue {
		labels := make(labels.Labels, 0, len(value.Metric))

		for k, v := range value.Metric {
			labels = append(labels, plabels.Label{
				Name:  string(k),
				Value: string(v),
			})
		}
		sort.Sort(labels)

		result = append(result, promql.Sample{
			Metric: labels,
			Point:  promql.Point{T: int64(value.Timestamp), V: float64(value.Value)},
		})
	}
	return result
}

// ImportRule imports the historical data for a single rule
func (importer *RuleImporter) ImportRule(ctx context.Context, rule rules.Rule) error {
	ts, err := parseTime(importer.config.Start)
	if err != nil {
		return err
	}
	end, err := parseTime(importer.config.End)
	if err != nil {
		return err
	}
	url, err := url.Parse(importer.config.URL)
	if err != nil {
		return err
	}

	appender := importer.headBlock.Appender() // use the multi writer?

	// todo: add recording rule label, etc
	vector, err := rule.Eval(ctx, ts, importer.queryFn, url)
	if err != nil {
		return err
	}
	ref, err := appender.Add(vector[0]., vector[0].T, vector[0].V)
	for ts.Before(end) {
		vector, err := rule.Eval(ctx, ts, importer.queryFn, url)
		if err != nil {
			return err
		}
		for x, t := range vector {
			// do stuff, don't AddFast because you need to maintain 
			// ref for each series bcs rule.Eval could return different labels,
			// so that means you would need to map the ref to metric, but that is what Add does 
			// anyways so just use that
		}



		for range vector { // todo: range over
			appender.AddFast(ref, vector[0].T, vector[0].V)
		}
		ts.Add(importer.config.EvalInterval)
	}
	return appender.Commit()
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.Errorf("cannot parse %q to a valid timestamp", s)
}

// CreateBlocks creates blocks for all the rule data
func (importer *RuleImporter) CreateBlocks() error {
	tmpdir, err := ioutil.TempDir("", "rule_blocks")
	if err != nil {
		return err
	}
	compactor, err := tsdb.NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{1000000}, nil)
	if err != nil {
		return err
	}
	os.MkdirAll(tmpdir, 0777)
	head := importer.headBlock
	_, err = compactor.Write(tmpdir, head, head.MinTime(), head.MaxTime()+1, nil)
	if err != nil {
		return err
	}
	return nil
}
