package watch

import (
	"context"
	"fmt"

	consulapi "github.com/hashicorp/consul/api"
)

// watchFactory is a function that can create a new WatchFunc
// from a parameter configuration
type watchFactory func(params map[string]interface{}) (WatcherFunc, error)

// watchFuncFactory maps each type to a factory function
var watchFuncFactory map[string]watchFactory

func init() {
	watchFuncFactory = map[string]watchFactory{
		"key":                  keyWatch,
		"keyprefix":            keyPrefixWatch,
		"services":             servicesWatch,
		"nodes":                nodesWatch,
		"service":              serviceWatch,
		"checks":               checksWatch,
		"event":                eventWatch,
		"connect_roots":        connectRootsWatch,
		"connect_leaf":         connectLeafWatch,
		"connect_proxy_config": connectProxyConfigWatch,
		"agent_service":        agentServiceWatch,
	}
}

// keyWatch is used to return a key watching function
func keyWatch(params map[string]interface{}) (WatcherFunc, error) {
	stale := false
	if err := assignValueBool(params, "stale", &stale); err != nil {
		return nil, err
	}

	var key string
	if err := assignValue(params, "key", &key); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, fmt.Errorf("Must specify a single key to watch")
	}
	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		kv := p.client.KV()
		opts := makeQueryOptionsWithContext(p, stale)
		defer p.cancelFunc()
		pair, meta, err := kv.Get(key, &opts)
		if err != nil {
			return nil, nil, err
		}
		if pair == nil {
			return WaitIndexVal(meta.LastIndex), nil, err
		}
		return WaitIndexVal(meta.LastIndex), pair, err
	}
	return fn, nil
}

// keyPrefixWatch is used to return a key prefix watching function
func keyPrefixWatch(params map[string]interface{}) (WatcherFunc, error) {
	stale := false
	if err := assignValueBool(params, "stale", &stale); err != nil {
		return nil, err
	}

	var prefix string
	if err := assignValue(params, "prefix", &prefix); err != nil {
		return nil, err
	}
	if prefix == "" {
		return nil, fmt.Errorf("Must specify a single prefix to watch")
	}
	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		kv := p.client.KV()
		opts := makeQueryOptionsWithContext(p, stale)
		defer p.cancelFunc()
		pairs, meta, err := kv.List(prefix, &opts)
		if err != nil {
			return nil, nil, err
		}
		return WaitIndexVal(meta.LastIndex), pairs, err
	}
	return fn, nil
}

// servicesWatch is used to watch the list of available services
func servicesWatch(params map[string]interface{}) (WatcherFunc, error) {
	stale := false
	if err := assignValueBool(params, "stale", &stale); err != nil {
		return nil, err
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		catalog := p.client.Catalog()
		opts := makeQueryOptionsWithContext(p, stale)
		defer p.cancelFunc()
		services, meta, err := catalog.Services(&opts)
		if err != nil {
			return nil, nil, err
		}
		return WaitIndexVal(meta.LastIndex), services, err
	}
	return fn, nil
}

// nodesWatch is used to watch the list of available nodes
func nodesWatch(params map[string]interface{}) (WatcherFunc, error) {
	stale := false
	if err := assignValueBool(params, "stale", &stale); err != nil {
		return nil, err
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		catalog := p.client.Catalog()
		opts := makeQueryOptionsWithContext(p, stale)
		defer p.cancelFunc()
		nodes, meta, err := catalog.Nodes(&opts)
		if err != nil {
			return nil, nil, err
		}
		return WaitIndexVal(meta.LastIndex), nodes, err
	}
	return fn, nil
}

// serviceWatch is used to watch a specific service for changes
func serviceWatch(params map[string]interface{}) (WatcherFunc, error) {
	stale := false
	if err := assignValueBool(params, "stale", &stale); err != nil {
		return nil, err
	}

	var (
		service string
		tags    []string
	)
	if err := assignValue(params, "service", &service); err != nil {
		return nil, err
	}
	if service == "" {
		return nil, fmt.Errorf("Must specify a single service to watch")
	}
	if err := assignValueStringSlice(params, "tag", &tags); err != nil {
		return nil, err
	}

	passingOnly := false
	if err := assignValueBool(params, "passingonly", &passingOnly); err != nil {
		return nil, err
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		health := p.client.Health()
		opts := makeQueryOptionsWithContext(p, stale)
		defer p.cancelFunc()
		nodes, meta, err := health.ServiceMultipleTags(service, tags, passingOnly, &opts)
		if err != nil {
			return nil, nil, err
		}
		return WaitIndexVal(meta.LastIndex), nodes, err
	}
	return fn, nil
}

// checksWatch is used to watch a specific checks in a given state
func checksWatch(params map[string]interface{}) (WatcherFunc, error) {
	stale := false
	if err := assignValueBool(params, "stale", &stale); err != nil {
		return nil, err
	}

	var service, state string
	if err := assignValue(params, "service", &service); err != nil {
		return nil, err
	}
	if err := assignValue(params, "state", &state); err != nil {
		return nil, err
	}
	if service != "" && state != "" {
		return nil, fmt.Errorf("Cannot specify service and state")
	}
	if service == "" && state == "" {
		state = "any"
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		health := p.client.Health()
		opts := makeQueryOptionsWithContext(p, stale)
		defer p.cancelFunc()
		var checks []*consulapi.HealthCheck
		var meta *consulapi.QueryMeta
		var err error
		if state != "" {
			checks, meta, err = health.State(state, &opts)
		} else {
			checks, meta, err = health.Checks(service, &opts)
		}
		if err != nil {
			return nil, nil, err
		}
		return WaitIndexVal(meta.LastIndex), checks, err
	}
	return fn, nil
}

// eventWatch is used to watch for events, optionally filtering on name
func eventWatch(params map[string]interface{}) (WatcherFunc, error) {
	// The stale setting doesn't apply to events.

	var name string
	if err := assignValue(params, "name", &name); err != nil {
		return nil, err
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		event := p.client.Event()
		opts := makeQueryOptionsWithContext(p, false)
		defer p.cancelFunc()
		events, meta, err := event.List(name, &opts)
		if err != nil {
			return nil, nil, err
		}

		// Prune to only the new events
		for i := 0; i < len(events); i++ {
			if WaitIndexVal(event.IDToIndex(events[i].ID)).Equal(p.lastParamVal) {
				events = events[i+1:]
				break
			}
		}
		return WaitIndexVal(meta.LastIndex), events, err
	}
	return fn, nil
}

// connectRootsWatch is used to watch for changes to Connect Root certificates.
func connectRootsWatch(params map[string]interface{}) (WatcherFunc, error) {
	// We don't support stale since roots are cached locally in the agent.

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		agent := p.client.Agent()
		opts := makeQueryOptionsWithContext(p, false)
		defer p.cancelFunc()

		roots, meta, err := agent.ConnectCARoots(&opts)
		if err != nil {
			return nil, nil, err
		}

		return WaitIndexVal(meta.LastIndex), roots, err
	}
	return fn, nil
}

// connectLeafWatch is used to watch for changes to Connect Leaf certificates
// for given local service id.
func connectLeafWatch(params map[string]interface{}) (WatcherFunc, error) {
	// We don't support stale since certs are cached locally in the agent.

	var serviceName string
	if err := assignValue(params, "service", &serviceName); err != nil {
		return nil, err
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		agent := p.client.Agent()
		opts := makeQueryOptionsWithContext(p, false)
		defer p.cancelFunc()

		leaf, meta, err := agent.ConnectCALeaf(serviceName, &opts)
		if err != nil {
			return nil, nil, err
		}

		return WaitIndexVal(meta.LastIndex), leaf, err
	}
	return fn, nil
}

// connectProxyConfigWatch is used to watch for changes to Connect managed proxy
// configuration. Note that this state is agent-local so the watch mechanism
// uses `hash` rather than `index` for deciding whether to block.
func connectProxyConfigWatch(params map[string]interface{}) (WatcherFunc, error) {
	// We don't support consistency modes since it's agent local data

	var proxyServiceID string
	if err := assignValue(params, "proxy_service_id", &proxyServiceID); err != nil {
		return nil, err
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		agent := p.client.Agent()
		opts := makeQueryOptionsWithContext(p, false)
		defer p.cancelFunc()

		config, _, err := agent.ConnectProxyConfig(proxyServiceID, &opts)
		if err != nil {
			return nil, nil, err
		}

		// Return string ContentHash since we don't have Raft indexes to block on.
		return WaitHashVal(config.ContentHash), config, err
	}
	return fn, nil
}

// agentServiceWatch is used to watch for changes to a single service instance
// on the local agent. Note that this state is agent-local so the watch
// mechanism uses `hash` rather than `index` for deciding whether to block.
func agentServiceWatch(params map[string]interface{}) (WatcherFunc, error) {
	// We don't support consistency modes since it's agent local data

	var serviceID string
	if err := assignValue(params, "service_id", &serviceID); err != nil {
		return nil, err
	}

	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		agent := p.client.Agent()
		opts := makeQueryOptionsWithContext(p, false)
		defer p.cancelFunc()

		svc, _, err := agent.Service(serviceID, &opts)
		if err != nil {
			return nil, nil, err
		}

		// Return string ContentHash since we don't have Raft indexes to block on.
		return WaitHashVal(svc.ContentHash), svc, err
	}
	return fn, nil
}

func makeQueryOptionsWithContext(p *Plan, stale bool) consulapi.QueryOptions {
	ctx, cancel := context.WithCancel(context.Background())
	p.setCancelFunc(cancel)
	opts := consulapi.QueryOptions{AllowStale: stale}
	switch param := p.lastParamVal.(type) {
	case WaitIndexVal:
		opts.WaitIndex = uint64(param)
	case WaitHashVal:
		opts.WaitHash = string(param)
	}
	return *opts.WithContext(ctx)
}
