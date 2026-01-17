package parser

// Options defines configuration for the PromQL parser.
type Options struct {
	EnableExperimentalFunctions  bool
	ExperimentalDurationEnabled  bool
	EnableExtendedRangeSelectors bool
}
