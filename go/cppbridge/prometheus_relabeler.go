package cppbridge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/common/model"
)

var (
	// ErrLSSNullPointer - error when lss is null pointer
	ErrLSSNullPointer = errors.New("lss is null pointer")
)

//
// Config for relabeling.
//

var (
	// RelabelTarget - validate Target label.
	RelabelTarget = regexp.MustCompile(`^(?:(?:[a-zA-Z_]|\$(?:\{\w+\}|\w+))+\w*)+$`)

	defaultRelabelConfig = RelabelConfig{
		Action:      Replace,
		Separator:   ";",
		Regex:       "(.*)",
		Replacement: "$1",
	}

	invalidTargetLabelForAction = "%q is invalid 'target_label' for %s action"
)

// RelabelConfig - is the configuration for relabeling of target label sets.
type RelabelConfig struct {
	// A list of labels from which values are taken and concatenated with the configured separator in order.
	SourceLabels []string `yaml:"source_labels,flow,omitempty"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `yaml:"separator,omitempty"`
	// Regex against which the concatenation is matched.
	Regex string `yaml:"regex,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `yaml:"modulus,omitempty"`
	// TargetLabel is the label to which the resulting string is written in a replacement.
	// Regexp interpolation is allowed for the replace action.
	TargetLabel string `yaml:"target_label,omitempty"`
	// Replacement is the regex replacement pattern to be used.
	Replacement string `yaml:"replacement,omitempty"`
	// Action is the action to be performed for the relabeling.
	Action Action `yaml:"action,omitempty"`
}

// UnmarshalYAML - implements the yaml.Unmarshaler interface.
func (c *RelabelConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = defaultRelabelConfig
	type plain RelabelConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Validate - validate config.
//
//revive:disable-next-line:cyclomatic this is validate.
//revive:disable-next-line:function-length long but readable.
//revive:disable-next-line:cognitive-complexity function is not complicated.
func (c *RelabelConfig) Validate() error {
	if c.Action == NoAction {
		return fmt.Errorf("relabel action cannot be empty")
	}
	if c.Modulus == 0 && c.Action == HashMod {
		return fmt.Errorf("relabel configuration for hashmod requires non-zero modulus")
	}
	if (c.Action == Replace ||
		c.Action == HashMod ||
		c.Action == Lowercase ||
		c.Action == Uppercase ||
		c.Action == KeepEqual ||
		c.Action == DropEqual) && c.TargetLabel == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'target_label' value", c.Action)
	}
	if c.Action == Replace && !strings.Contains(c.TargetLabel, "$") && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}
	if c.Action == Replace && strings.Contains(c.TargetLabel, "$") && !RelabelTarget.MatchString(c.TargetLabel) {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase ||
		c.Action == Uppercase ||
		c.Action == KeepEqual ||
		c.Action == DropEqual) && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase ||
		c.Action == Uppercase ||
		c.Action == KeepEqual ||
		c.Action == DropEqual) && c.Replacement != defaultRelabelConfig.Replacement {
		return fmt.Errorf("'replacement' can not be set for %s action", c.Action)
	}
	if c.Action == LabelMap && !RelabelTarget.MatchString(c.Replacement) {
		return fmt.Errorf("%q is invalid 'replacement' for %s action", c.Replacement, c.Action)
	}
	if c.Action == HashMod && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}

	if c.Action == DropEqual || c.Action == KeepEqual {
		if c.Regex != defaultRelabelConfig.Regex ||
			c.Modulus != defaultRelabelConfig.Modulus ||
			c.Separator != defaultRelabelConfig.Separator ||
			c.Replacement != defaultRelabelConfig.Replacement {
			return fmt.Errorf(
				"%s action requires only 'source_labels' and `target_label`, and no other fields",
				c.Action,
			)
		}
	}

	if c.Action == LabelDrop || c.Action == LabelKeep {
		if c.SourceLabels != nil ||
			c.TargetLabel != defaultRelabelConfig.TargetLabel ||
			c.Modulus != defaultRelabelConfig.Modulus ||
			c.Separator != defaultRelabelConfig.Separator ||
			c.Replacement != defaultRelabelConfig.Replacement {
			return fmt.Errorf("%s action requires only 'regex', and no other fields", c.Action)
		}
	}

	return nil
}

// Equal check for complete coincidence of values.
func (c *RelabelConfig) Equal(input *RelabelConfig) bool {
	if len(c.SourceLabels) != len(input.SourceLabels) {
		return false
	}

	for j := range c.SourceLabels {
		if c.SourceLabels[j] != input.SourceLabels[j] {
			return false
		}
	}

	if c.Separator != input.Separator {
		return false
	}

	if c.Regex != input.Regex {
		return false
	}

	if c.Modulus != input.Modulus {
		return false
	}

	if c.TargetLabel != input.TargetLabel {
		return false
	}

	if c.Replacement != input.Replacement {
		return false
	}

	if c.Action != input.Action {
		return false
	}

	return true
}

// Copy return copy *RelabelConfig.
func (c *RelabelConfig) Copy() *RelabelConfig {
	newCfg := &RelabelConfig{
		SourceLabels: make([]string, 0, len(c.SourceLabels)),
		Separator:    c.Separator,
		Regex:        c.Regex,
		Modulus:      c.Modulus,
		TargetLabel:  c.TargetLabel,
		Replacement:  c.Replacement,
		Action:       c.Action,
	}
	newCfg.SourceLabels = append(newCfg.SourceLabels, c.SourceLabels...)
	return newCfg
}

// Action - is the action to be performed on relabeling.
type Action uint8

const (
	// NoAction - no action, init state.
	NoAction Action = iota
	// Drop - drops targets for which the input does match the regex.
	Drop
	// Keep - drops targets for which the input does not match the regex.
	Keep
	// DropEqual - drops targets for which the input does match the target.
	DropEqual
	// KeepEqual - drops targets for which the input does not match the target.
	KeepEqual
	// Replace - performs a regex replacement.
	Replace
	// Lowercase - maps input letters to their lower case.
	Lowercase
	// Uppercase - maps input letters to their upper case.
	Uppercase
	// HashMod - sets a label to the modulus of a hash of labels.
	HashMod
	// LabelMap - copies labels to other labelnames based on a regex.
	LabelMap
	// LabelDrop - drops any label matching the regex.
	LabelDrop
	// LabelKeep - drops any label not matching the regex.
	LabelKeep
)

// actionNameToValueMap - converting Action string name to Action value.
var actionNameToValueMap = map[string]Action{
	"drop":      Drop,
	"keep":      Keep,
	"dropequal": DropEqual,
	"keepequal": KeepEqual,
	"replace":   Replace,
	"lowercase": Lowercase,
	"uppercase": Uppercase,
	"hashmod":   HashMod,
	"labelmap":  LabelMap,
	"labeldrop": LabelDrop,
	"labelkeep": LabelKeep,
}

// actionValueToNameMap - converting Action value to Action string name.
var actionValueToNameMap = map[Action]string{
	Drop:      "drop",
	Keep:      "keep",
	DropEqual: "dropequal",
	KeepEqual: "keepequal",
	Replace:   "replace",
	Lowercase: "lowercase",
	Uppercase: "uppercase",
	HashMod:   "hashmod",
	LabelMap:  "labelmap",
	LabelDrop: "labeldrop",
	LabelKeep: "labelkeep",
}

// String - serialize to string.
func (a Action) String() string {
	v, ok := actionValueToNameMap[a]
	if !ok {
		return fmt.Sprintf("Action(%d)", a)
	}

	return v
}

// UnmarshalYAML - implements the yaml.Unmarshaler interface.
func (a *Action) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	v, ok := actionNameToValueMap[strings.ToLower(s)]
	if !ok {
		return fmt.Errorf("unknown relabel action %q", s)
	}
	*a = v

	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (a Action) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

// ToHash make hash from RelabelConfig's.
func ToHash(rCfgs []*RelabelConfig) uint64 {
	h := xxhash.New()
	for _, rcfg := range rCfgs {
		for _, sl := range rcfg.SourceLabels {
			_, _ = h.WriteString(sl)
		}
		_, _ = h.WriteString(rcfg.Separator)
		_, _ = h.WriteString(rcfg.Regex)
		_, _ = h.WriteString(strconv.FormatUint(rcfg.Modulus, 10)) //revive:disable-line:add-constant it's base 10
		_, _ = h.WriteString(rcfg.TargetLabel)
		_, _ = h.WriteString(rcfg.Replacement)
		_, _ = h.WriteString(rcfg.Action.String())
	}

	return h.Sum64()
}

//
// StatelessRelabeler
//

// StatelessRelabeler - go wrapper for C-StatelessRelabeler.
//
//	cptr - pointer to a C++ StatelessRelabeler initiated in C++ memory;
type StatelessRelabeler struct {
	rCfgs      []*RelabelConfig
	cptr       uintptr
	generation uint64
}

// NewStatelessRelabeler - init new StatelessRelabeler.
func NewStatelessRelabeler(rCfgs []*RelabelConfig) (*StatelessRelabeler, error) {
	cptr, exception := prometheusStatelessRelabelerCtor(rCfgs)
	if len(exception) != 0 {
		return nil, handleException(exception)
	}
	sr := &StatelessRelabeler{
		cptr:       cptr,
		rCfgs:      rCfgs,
		generation: ToHash(rCfgs),
	}
	runtime.SetFinalizer(sr, func(cr *StatelessRelabeler) {
		prometheusStatelessRelabelerDtor(cr.cptr)
		cr.rCfgs = nil
	})
	return sr, nil
}

// Pointer - return c-pointer.
func (sr *StatelessRelabeler) Pointer() uintptr {
	return sr.cptr
}

// Generation return StatelessRelabeler's generation hash from configs.
func (sr *StatelessRelabeler) Generation() uint64 {
	return sr.generation
}

// EqualConfigs check for complete matching of configs.
func (sr *StatelessRelabeler) EqualConfigs(relabelingCfgs []*RelabelConfig) bool {
	if len(sr.rCfgs) != len(relabelingCfgs) {
		return false
	}

	for i := range sr.rCfgs {
		if !sr.rCfgs[i].Equal(relabelingCfgs[i]) {
			return false
		}
	}

	return true
}

// ResetTo reset configs and replace on new converting go-config.
func (sr *StatelessRelabeler) ResetTo(relabelingCfgs []*RelabelConfig) error {
	sr.rCfgs = relabelingCfgs
	sr.generation = ToHash(relabelingCfgs)
	exception := prometheusStatelessRelabelerResetTo(sr.cptr, sr.rCfgs)
	return handleException(exception)
}

//
// ShardsInnerSeries
//

// NewShardsInnerSeries - init slice with the results of relabeling per shards.
func NewShardsInnerSeries(numberOfShards uint16) []*InnerSeries {
	srs := make([]*InnerSeries, numberOfShards)
	for i := range srs {
		srs[i] = NewInnerSeries()
	}

	return srs
}

// stdVector implementation cpp std::vector, for allocate 24-byte, used in cpp.
//
//nolint:unused // for cpp-bridge, used in cpp.
type stdVector struct {
	start        uintptr
	finish       uintptr
	endOfStorage uintptr
}

// InnerSeries - go wrapper for C-InnerSeries.
//
//	size - number of timeseries processed;
//	data - pointer for vector with timeseries;
type InnerSeries struct {
	size uint64
	//nolint:unused // for cpp-bridge, used in cpp
	data stdVector
}

// Size - number of Timeseries.
func (iss *InnerSeries) Size() uint64 {
	return iss.size
}

// NewInnerSeries - init new InnerSeries with finalizer for dtor C-InnerSeries.
func NewInnerSeries() *InnerSeries {
	rts := &InnerSeries{size: 0}
	prometheusInnerSeriesCtor(rts)
	runtime.SetFinalizer(rts, func(r *InnerSeries) {
		prometheusInnerSeriesDtor(r)
	})

	return rts
}

//
// ShardsRelabeledSeries
//

// NewShardsRelabeledSeries - init slice with the relabeled results per shards.
func NewShardsRelabeledSeries(numberOfShards uint16) []*RelabeledSeries {
	rrs := make([]*RelabeledSeries, numberOfShards)
	for i := range rrs {
		rrs[i] = NewRelabeledSeries()
	}

	return rrs
}

// RelabeledSeries - go wrapper for C-RelabeledSeries.
//
//	size - number of relabeled elements processed;
//	data - pointer for vector with relabeled elements;
type RelabeledSeries struct {
	size uint64
	//nolint:unused // for cpp-bridge, used in cpp
	data stdVector
}

// NewRelabeledSeries - init new RelabeledSeries with finalizer for dtor C-RelabeledSeries.
func NewRelabeledSeries() *RelabeledSeries {
	rss := &RelabeledSeries{size: 0}
	prometheusRelabeledSeriesCtor(rss)
	runtime.SetFinalizer(rss, func(r *RelabeledSeries) {
		prometheusRelabeledSeriesDtor(r)
	})

	return rss
}

// Size - number of series.
func (rss *RelabeledSeries) Size() uint64 {
	return rss.size
}

// RelabelerStateUpdate - go wrapper for C-RelabelerStateUpdate.
//
//	data - pointer for vector with relabeled elements;
type RelabelerStateUpdate struct {
	//nolint:unused // for cpp-bridge, used in cpp
	data stdVector
}

// NewRelabelerStateUpdate - init new RelabelerStateUpdate.
func NewRelabelerStateUpdate() *RelabelerStateUpdate {
	ud := new(RelabelerStateUpdate)
	prometheusRelabelerStateUpdateCtor(ud)
	runtime.SetFinalizer(ud, func(r *RelabelerStateUpdate) {
		prometheusRelabelerStateUpdateDtor(r)
	})

	return ud
}

// MetricLimits limits on label set and samples.
type MetricLimits struct {
	LabelLimit            int64
	LabelNameLengthLimit  int64
	LabelValueLengthLimit int64
	SampleLimit           int64
}

// RelabelerOptions relabeling options.
type RelabelerOptions struct {
	TargetLabels []Label
	MetricLimits *MetricLimits
	HonorLabels  bool
}

// StaleNansState wrap pointer to source state for stale nans .
type StaleNansState struct {
	state uintptr
}

// NewStaleNansState init new SourceStaleNansState.
func NewStaleNansState() *StaleNansState {
	s := &StaleNansState{
		state: prometheusRelabelStaleNansStateCtor(),
	}
	runtime.SetFinalizer(s, func(s *StaleNansState) {
		prometheusRelabelStaleNansStateDtor(s.state)
	})

	return s
}

func (s *StaleNansState) Reset() {
	prometheusRelabelStaleNansStateReset(s.state)
}

// InputPerShardRelabeler - go wrapper for C-PerShardRelabeler, relabeler for shard.
//
//	cptr               - pointer to C-InputPerShardRelabeler;
//	lss                - pointer to go LSS, keep alive for gc;
//	statelessRelabeler - pointer to go StatelessRelabeler, for keep alive;
//	shardID            - current shard id;
//	numberOfShards     - total shards count;
type InputPerShardRelabeler struct {
	statelessRelabeler *StatelessRelabeler
	cptr               uintptr
	shardID            uint16
	numberOfShards     uint16
}

// NewInputPerShardRelabeler - init new InputPerShardRelabeler.
func NewInputPerShardRelabeler(
	statelessRelabeler *StatelessRelabeler,
	numberOfShards, shardID uint16,
) (*InputPerShardRelabeler, error) {
	p, exception := prometheusPerShardRelabelerCtor(
		nil,
		statelessRelabeler.Pointer(),
		numberOfShards,
		shardID,
	)
	if len(exception) != 0 {
		return nil, handleException(exception)
	}

	ipsr := &InputPerShardRelabeler{
		cptr:               p,
		statelessRelabeler: statelessRelabeler,
		shardID:            shardID,
		numberOfShards:     numberOfShards,
	}
	runtime.SetFinalizer(ipsr, func(psr *InputPerShardRelabeler) {
		prometheusPerShardRelabelerDtor(psr.cptr)
		psr.statelessRelabeler = nil
	})
	return ipsr, nil
}

// AppendRelabelerSeries - add relabeled ls to lss, add to result and add to cache update(second stage).
func (ipsr *InputPerShardRelabeler) AppendRelabelerSeries(
	ctx context.Context,
	lss *LabelSetStorage,
	relabelerStateUpdate *RelabelerStateUpdate,
	innerSeries *InnerSeries,
	relabeledSeries *RelabeledSeries,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exception := prometheusPerShardRelabelerAppendRelabelerSeries(
		ipsr.cptr,
		lss.Pointer(),
		innerSeries,
		relabeledSeries,
		relabelerStateUpdate,
	)

	return handleException(exception)
}

// CacheAllocatedMemory - return size of allocated memory for cache map.
func (ipsr *InputPerShardRelabeler) CacheAllocatedMemory() uint64 {
	return prometheusPerShardRelabelerCacheAllocatedMemory(ipsr.cptr)
}

// Generation return current statelessRelabeler generation.
func (ipsr *InputPerShardRelabeler) Generation() uint64 {
	return ipsr.statelessRelabeler.Generation()
}

// InputRelabeling - relabeling incoming hashdex(first stage).
func (ipsr *InputPerShardRelabeler) InputRelabeling(
	ctx context.Context,
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	cache *Cache,
	options RelabelerOptions,
	shardedData ShardedData,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cptrContainer, ok := shardedData.(cptrable)
	if !ok {
		return ErrMustImplementCptrable
	}
	exception := prometheusPerShardRelabelerInputRelabeling(
		ipsr.cptr,
		inputLss.Pointer(),
		targetLss.Pointer(),
		cache.cPointer,
		cptrContainer.cptr(),
		options,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)

	return handleException(exception)
}

// InputRelabelingWithStalenans relabeling incoming hashdex(first stage) with state stalenans.
func (ipsr *InputPerShardRelabeler) InputRelabelingWithStalenans(
	ctx context.Context,
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	cache *Cache,
	options RelabelerOptions,
	staleNansState *StaleNansState,
	staleNansTS int64,
	shardedData ShardedData,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cptrContainer, ok := shardedData.(cptrable)
	if !ok {
		return ErrMustImplementCptrable
	}
	exception := prometheusPerShardRelabelerInputRelabelingWithStalenans(
		ipsr.cptr,
		inputLss.Pointer(),
		targetLss.Pointer(),
		cache.cPointer,
		cptrContainer.cptr(),
		staleNansState.state,
		staleNansTS,
		options,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	if exception != nil {
		return handleException(exception)
	}

	return nil
}

// NumberOfShards return current numberOfShards.
func (ipsr *InputPerShardRelabeler) NumberOfShards() uint16 {
	return ipsr.numberOfShards
}

// ResetTo - set new number_of_shards and external_labels.
func (ipsr *InputPerShardRelabeler) ResetTo(numberOfShards uint16) {
	ipsr.numberOfShards = numberOfShards
	prometheusPerShardRelabelerResetTo(nil, ipsr.cptr, ipsr.numberOfShards)
}

// StatelessRelabeler return current *StatelessRelabeler.
func (ipsr *InputPerShardRelabeler) StatelessRelabeler() *StatelessRelabeler {
	return ipsr.statelessRelabeler
}

// UpdateRelabelerState - add to cache relabled data(third stage).
func (ipsr *InputPerShardRelabeler) UpdateRelabelerState(
	ctx context.Context,
	cache *Cache,
	relabelerStateUpdate *RelabelerStateUpdate,
	relabeledShardID uint16,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exception := prometheusPerShardRelabelerUpdateRelabelerState(
		relabelerStateUpdate,
		ipsr.cptr,
		cache.cPointer,
		relabeledShardID,
	)

	return handleException(exception)
}

// OutputPerShardRelabeler go wrapper for C-PerShardRelabeler, relabeler for shard.
//
//	p                  - pointer to C-InputPerShardRelabeler;
//	lss                - pointer to go LSS, keep alive for gc;
//	statelessRelabeler - pointer to go StatelessRelabeler, for keep alive;
//	shardID            - current shard id;
//	logShards          - logarithm to the base 2 of total shards count(encoders);
type OutputPerShardRelabeler struct {
	statelessRelabeler           *StatelessRelabeler
	cache                        *Cache
	cptr                         uintptr
	generationStatelessRelabeler uint64
	generationManagerKeeper      uint32
	numberOfShards               uint16
	shardID                      uint16
}

// NewOutputPerShardRelabeler init new OutputPerShardRelabeler.
func NewOutputPerShardRelabeler(
	externalLabels []Label,
	statelessRelabeler *StatelessRelabeler,
	generationManagerKeeper uint32,
	numberOfShards, shardID uint16,
) (*OutputPerShardRelabeler, error) {
	p, exception := prometheusPerShardRelabelerCtor(
		externalLabels,
		statelessRelabeler.Pointer(),
		shardID,
		numberOfShards,
	)
	if len(exception) != 0 {
		return nil, handleException(exception)
	}

	opsr := &OutputPerShardRelabeler{
		statelessRelabeler:           statelessRelabeler,
		cache:                        NewCache(),
		cptr:                         p,
		generationStatelessRelabeler: statelessRelabeler.Generation(),
		generationManagerKeeper:      generationManagerKeeper,
		numberOfShards:               numberOfShards,
		shardID:                      shardID,
	}
	runtime.SetFinalizer(opsr, func(psr *OutputPerShardRelabeler) {
		prometheusPerShardRelabelerDtor(psr.cptr)
		psr.statelessRelabeler = nil
	})
	return opsr, nil
}

// CacheAllocatedMemory return size of allocated memory for cache map.
func (opsr *OutputPerShardRelabeler) CacheAllocatedMemory() uint64 {
	return prometheusPerShardRelabelerCacheAllocatedMemory(opsr.cptr)
}

// OutputRelabeling relabeling output series(fourth stage).
func (opsr *OutputPerShardRelabeler) OutputRelabeling(
	ctx context.Context,
	lss *LabelSetStorage,
	incomingInnerSeries []*InnerSeries,
	encodersInnerSeries []*InnerSeries,
	relabeledSeries *RelabeledSeries,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exception := prometheusPerShardRelabelerOutputRelabeling(
		opsr.cptr,
		lss.Pointer(),
		opsr.cache.cPointer,
		incomingInnerSeries,
		encodersInnerSeries,
		relabeledSeries,
	)

	return handleException(exception)
}

// ResetCache reset cache if need.
func (opsr *OutputPerShardRelabeler) ResetCache(generationManagerKeeper uint32, numberOfShards uint16) {
	if opsr.generationStatelessRelabeler == opsr.statelessRelabeler.Generation() &&
		opsr.generationManagerKeeper == generationManagerKeeper &&
		opsr.numberOfShards == numberOfShards {
		return
	}
	opsr.cache = NewCache()
}

// ResetTo reset set new number_of_shards and external_labels.
func (opsr *OutputPerShardRelabeler) ResetTo(
	externalLabels []Label,
	generationManagerKeeper uint32,
	numberOfShards uint16,
) {
	opsr.ResetCache(generationManagerKeeper, numberOfShards)
	opsr.numberOfShards = numberOfShards
	opsr.generationManagerKeeper = generationManagerKeeper
	prometheusPerShardRelabelerResetTo(externalLabels, opsr.cptr, opsr.numberOfShards)
}

// StatelessRelabeler return current *StatelessRelabeler.
func (opsr *OutputPerShardRelabeler) StatelessRelabeler() *StatelessRelabeler {
	return opsr.statelessRelabeler
}

// UpdateRelabelerState add to cache relabled data(fifth stage).
func (opsr *OutputPerShardRelabeler) UpdateRelabelerState(
	ctx context.Context,
	relabelerStateUpdate *RelabelerStateUpdate,
	relabeledShardID uint16,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exception := prometheusPerShardRelabelerUpdateRelabelerState(
		relabelerStateUpdate,
		opsr.cptr,
		opsr.cache.cPointer,
		relabeledShardID,
	)

	return handleException(exception)
}

// Label is a key/value pair of strings.
type Label struct {
	Name  string
	Value string
}

//
// Cache
//

// Cache - go wrapper for C-Cache, cache for relabeler.
//
//	cPointer   - pointer to C-Cache;
type Cache struct {
	cPointer uintptr
}

// NewCache init new Cache.
func NewCache() *Cache {
	cache := &Cache{
		cPointer: prometheusCacheCtor(),
	}
	runtime.SetFinalizer(cache, func(c *Cache) {
		prometheusCacheDtor(c.cPointer)
	})
	return cache
}

// AllocatedMemory return size of allocated memory for caches.
func (c *Cache) AllocatedMemory() uint64 {
	return prometheusCacheAllocatedMemory(c.cPointer)
}

// ResetTo reset cache.
func (c *Cache) ResetTo() {
	prometheusCacheResetTo(c.cPointer)
}

// State state of relabelers per shard.
type State struct {
	// relabelerID string
	caches              []*Cache
	staleNansStates     []*StaleNansState
	staleNansTS         int64
	generationRelabeler uint64
	generationHead      uint64
	options             RelabelerOptions
	trackStaleness      bool
}

// NewState init new State.
func NewState(numberOfShards uint16) *State {
	s := &State{
		caches:              make([]*Cache, numberOfShards),
		staleNansStates:     make([]*StaleNansState, numberOfShards),
		generationRelabeler: math.MaxUint64,
		generationHead:      math.MaxUint64,
		trackStaleness:      false,
	}

	return s
}

// CacheByShard return *Cache for shard.
func (s *State) CacheByShard(shardID uint16) *Cache {
	if int(shardID) >= len(s.caches) {
		panic(fmt.Sprintf(
			"shardID(%d) out of range in caches(%d)",
			shardID,
			len(s.caches),
		))
	}

	if s.caches[shardID] == nil {
		s.caches[shardID] = NewCache()
	}

	return s.caches[shardID]
}

// StaleNansStateByShard return SourceStaleNansState for shard.
func (s *State) StaleNansStateByShard(shardID uint16) *StaleNansState {
	if int(shardID) >= len(s.staleNansStates) {
		panic(fmt.Sprintf(
			"shardID(%d) out of range in staleNansStates(%d)",
			shardID,
			len(s.caches),
		))
	}

	if s.staleNansStates[shardID] == nil {
		s.staleNansStates[shardID] = NewStaleNansState()
	}

	return s.staleNansStates[shardID]
}

// StaleNansTS return timestamp for stalenan.
func (s *State) StaleNansTS() int64 {
	if s.staleNansTS == 0 {
		return time.Now().UnixMilli()
	}

	return s.staleNansTS
}

// SetStaleNansTS set timestamp for stalenan.
func (s *State) SetStaleNansTS(ts int64) {
	s.staleNansTS = ts
}

// EnableTrackStaleness enable track stalenans.
func (s *State) EnableTrackStaleness() {
	s.trackStaleness = true
}

// DisableTrackStaleness disable track stalenans.
func (s *State) DisableTrackStaleness() {
	s.trackStaleness = false
}

// TrackStaleness return state track stalenans.
func (s *State) TrackStaleness() bool {
	return s.trackStaleness
}

// RelabelerOptions return Options for relabeler.
func (s *State) RelabelerOptions() RelabelerOptions {
	return s.options
}

// SetRelabelerOptions set Options for relabeler.
func (s *State) SetRelabelerOptions(options *RelabelerOptions) {
	s.options = *options
}

// Reconfigure recreate caches and stalenans states if need and set new generations.
func (s *State) Reconfigure(
	generationRelabeler uint64,
	generationHead uint64,
	numberOfShards uint16,
) {
	equaledGeneration := generationRelabeler == s.generationRelabeler &&
		generationHead == s.generationHead
	s.resetCaches(numberOfShards, equaledGeneration)
	s.resetStaleNansStates(numberOfShards, equaledGeneration)
	s.generationRelabeler = generationRelabeler
	s.generationHead = generationHead
}

// resetCaches recreate Caches.
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func (s *State) resetCaches(numberOfShards uint16, equaledGeneration bool) {
	if equaledGeneration && int(numberOfShards) == len(s.caches) {
		return
	}

	for shardID := range s.caches {
		s.caches[shardID] = nil
	}

	if len(s.caches) > int(numberOfShards) {
		// cut
		s.caches = s.caches[:numberOfShards]
	}

	if len(s.caches) < int(numberOfShards) {
		// grow
		s.caches = append(
			s.caches,
			make([]*Cache, int(numberOfShards)-len(s.caches))...,
		)
	}
}

// resetStaleNansStates recreate StaleNansStates.
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func (s *State) resetStaleNansStates(numberOfShards uint16, equaledGeneration bool) {
	if !s.trackStaleness {
		return
	}

	if equaledGeneration && int(numberOfShards) == len(s.staleNansStates) {
		return
	}

	for shardID := range s.staleNansStates {
		state := s.staleNansStates[shardID]
		if state != nil {
			state.Reset()
		}
	}

	if len(s.staleNansStates) > int(numberOfShards) {
		// cut
		s.staleNansStates = s.staleNansStates[:numberOfShards]
	}

	if len(s.staleNansStates) < int(numberOfShards) {
		// grow
		s.staleNansStates = append(
			s.staleNansStates,
			make([]*StaleNansState, int(numberOfShards)-len(s.staleNansStates))...,
		)
	}
}
