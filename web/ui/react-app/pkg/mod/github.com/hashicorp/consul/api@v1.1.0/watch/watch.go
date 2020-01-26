package watch

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/mitchellh/mapstructure"
)

const DefaultTimeout = 10 * time.Second

// Plan is the parsed version of a watch specification. A watch provides
// the details of a query, which generates a view into the Consul data store.
// This view is watched for changes and a handler is invoked to take any
// appropriate actions.
type Plan struct {
	Datacenter  string
	Token       string
	Type        string
	HandlerType string
	Exempt      map[string]interface{}

	Watcher WatcherFunc
	// Handler is kept for backward compatibility but only supports watches based
	// on index param. To support hash based watches, set HybridHandler instead.
	Handler       HandlerFunc
	HybridHandler HybridHandlerFunc
	LogOutput     io.Writer

	address      string
	client       *consulapi.Client
	lastParamVal BlockingParamVal
	lastResult   interface{}

	stop       bool
	stopCh     chan struct{}
	stopLock   sync.Mutex
	cancelFunc context.CancelFunc
}

type HttpHandlerConfig struct {
	Path          string              `mapstructure:"path"`
	Method        string              `mapstructure:"method"`
	Timeout       time.Duration       `mapstructure:"-"`
	TimeoutRaw    string              `mapstructure:"timeout"`
	Header        map[string][]string `mapstructure:"header"`
	TLSSkipVerify bool                `mapstructure:"tls_skip_verify"`
}

// BlockingParamVal is an interface representing the common operations needed for
// different styles of blocking. It's used to abstract the core watch plan from
// whether we are performing index-based or hash-based blocking.
type BlockingParamVal interface {
	// Equal returns whether the other param value should be considered equal
	// (i.e. representing no change in the watched resource). Equal must not panic
	// if other is nil.
	Equal(other BlockingParamVal) bool

	// Next is called when deciding which value to use on the next blocking call.
	// It assumes the BlockingParamVal value it is called on is the most recent one
	// returned and passes the previous one which may be nil as context. This
	// allows types to customize logic around ordering without assuming there is
	// an order. For example WaitIndexVal can check that the index didn't go
	// backwards and if it did then reset to 0. Most other cases should just
	// return themselves (the most recent value) to be used in the next request.
	Next(previous BlockingParamVal) BlockingParamVal
}

// WaitIndexVal is a type representing a Consul index that implements
// BlockingParamVal.
type WaitIndexVal uint64

// Equal implements BlockingParamVal
func (idx WaitIndexVal) Equal(other BlockingParamVal) bool {
	if otherIdx, ok := other.(WaitIndexVal); ok {
		return idx == otherIdx
	}
	return false
}

// Next implements BlockingParamVal
func (idx WaitIndexVal) Next(previous BlockingParamVal) BlockingParamVal {
	if previous == nil {
		return idx
	}
	prevIdx, ok := previous.(WaitIndexVal)
	if ok && prevIdx > idx {
		// This value is smaller than the previous index, reset.
		return WaitIndexVal(0)
	}
	return idx
}

// WaitHashVal is a type representing a Consul content hash that implements
// BlockingParamVal.
type WaitHashVal string

// Equal implements BlockingParamVal
func (h WaitHashVal) Equal(other BlockingParamVal) bool {
	if otherHash, ok := other.(WaitHashVal); ok {
		return h == otherHash
	}
	return false
}

// Next implements BlockingParamVal
func (h WaitHashVal) Next(previous BlockingParamVal) BlockingParamVal {
	return h
}

// WatcherFunc is used to watch for a diff.
type WatcherFunc func(*Plan) (BlockingParamVal, interface{}, error)

// HandlerFunc is used to handle new data. It only works for index-based watches
// (which is almost all end points currently) and is kept for backwards
// compatibility until more places can make use of hash-based watches too.
type HandlerFunc func(uint64, interface{})

// HybridHandlerFunc is used to handle new data. It can support either
// index-based or hash-based watches via the BlockingParamVal.
type HybridHandlerFunc func(BlockingParamVal, interface{})

// Parse takes a watch query and compiles it into a WatchPlan or an error
func Parse(params map[string]interface{}) (*Plan, error) {
	return ParseExempt(params, nil)
}

// ParseExempt takes a watch query and compiles it into a WatchPlan or an error
// Any exempt parameters are stored in the Exempt map
func ParseExempt(params map[string]interface{}, exempt []string) (*Plan, error) {
	plan := &Plan{
		stopCh: make(chan struct{}),
		Exempt: make(map[string]interface{}),
	}

	// Parse the generic parameters
	if err := assignValue(params, "datacenter", &plan.Datacenter); err != nil {
		return nil, err
	}
	if err := assignValue(params, "token", &plan.Token); err != nil {
		return nil, err
	}
	if err := assignValue(params, "type", &plan.Type); err != nil {
		return nil, err
	}
	// Ensure there is a watch type
	if plan.Type == "" {
		return nil, fmt.Errorf("Watch type must be specified")
	}

	// Get the specific handler
	if err := assignValue(params, "handler_type", &plan.HandlerType); err != nil {
		return nil, err
	}
	switch plan.HandlerType {
	case "http":
		if _, ok := params["http_handler_config"]; !ok {
			return nil, fmt.Errorf("Handler type 'http' requires 'http_handler_config' to be set")
		}
		config, err := parseHttpHandlerConfig(params["http_handler_config"])
		if err != nil {
			return nil, fmt.Errorf(fmt.Sprintf("Failed to parse 'http_handler_config': %v", err))
		}
		plan.Exempt["http_handler_config"] = config
		delete(params, "http_handler_config")

	case "script":
		// Let the caller check for configuration in exempt parameters
	}

	// Look for a factory function
	factory := watchFuncFactory[plan.Type]
	if factory == nil {
		return nil, fmt.Errorf("Unsupported watch type: %s", plan.Type)
	}

	// Get the watch func
	fn, err := factory(params)
	if err != nil {
		return nil, err
	}
	plan.Watcher = fn

	// Remove the exempt parameters
	if len(exempt) > 0 {
		for _, ex := range exempt {
			val, ok := params[ex]
			if ok {
				plan.Exempt[ex] = val
				delete(params, ex)
			}
		}
	}

	// Ensure all parameters are consumed
	if len(params) != 0 {
		var bad []string
		for key := range params {
			bad = append(bad, key)
		}
		return nil, fmt.Errorf("Invalid parameters: %v", bad)
	}
	return plan, nil
}

// assignValue is used to extract a value ensuring it is a string
func assignValue(params map[string]interface{}, name string, out *string) error {
	if raw, ok := params[name]; ok {
		val, ok := raw.(string)
		if !ok {
			return fmt.Errorf("Expecting %s to be a string", name)
		}
		*out = val
		delete(params, name)
	}
	return nil
}

// assignValueBool is used to extract a value ensuring it is a bool
func assignValueBool(params map[string]interface{}, name string, out *bool) error {
	if raw, ok := params[name]; ok {
		val, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("Expecting %s to be a boolean", name)
		}
		*out = val
		delete(params, name)
	}
	return nil
}

// assignValueStringSlice is used to extract a value ensuring it is either a string or a slice of strings
func assignValueStringSlice(params map[string]interface{}, name string, out *[]string) error {
	if raw, ok := params[name]; ok {
		var tmp []string
		switch raw.(type) {
		case string:
			tmp = make([]string, 1, 1)
			tmp[0] = raw.(string)
		case []string:
			l := len(raw.([]string))
			tmp = make([]string, l, l)
			copy(tmp, raw.([]string))
		case []interface{}:
			l := len(raw.([]interface{}))
			tmp = make([]string, l, l)
			for i, v := range raw.([]interface{}) {
				if s, ok := v.(string); ok {
					tmp[i] = s
				} else {
					return fmt.Errorf("Index %d of %s expected to be string", i, name)
				}
			}
		default:
			return fmt.Errorf("Expecting %s to be a string or []string", name)
		}
		*out = tmp
		delete(params, name)
	}
	return nil
}

// Parse the 'http_handler_config' parameters
func parseHttpHandlerConfig(configParams interface{}) (*HttpHandlerConfig, error) {
	var config HttpHandlerConfig
	if err := mapstructure.Decode(configParams, &config); err != nil {
		return nil, err
	}

	if config.Path == "" {
		return nil, fmt.Errorf("Requires 'path' to be set")
	}
	if config.Method == "" {
		config.Method = "POST"
	}
	if config.TimeoutRaw == "" {
		config.Timeout = DefaultTimeout
	} else if timeout, err := time.ParseDuration(config.TimeoutRaw); err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("Failed to parse timeout: %v", err))
	} else {
		config.Timeout = timeout
	}

	return &config, nil
}
