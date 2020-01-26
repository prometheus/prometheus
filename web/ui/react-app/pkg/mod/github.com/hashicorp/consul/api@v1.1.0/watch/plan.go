package watch

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

const (
	// retryInterval is the base retry value
	retryInterval = 5 * time.Second

	// maximum back off time, this is to prevent
	// exponential runaway
	maxBackoffTime = 180 * time.Second
)

func (p *Plan) Run(address string) error {
	return p.RunWithConfig(address, nil)
}

// Run is used to run a watch plan
func (p *Plan) RunWithConfig(address string, conf *consulapi.Config) error {
	// Setup the client
	p.address = address
	if conf == nil {
		conf = consulapi.DefaultConfig()
	}
	conf.Address = address
	conf.Datacenter = p.Datacenter
	conf.Token = p.Token
	client, err := consulapi.NewClient(conf)
	if err != nil {
		return fmt.Errorf("Failed to connect to agent: %v", err)
	}

	// Create the logger
	output := p.LogOutput
	if output == nil {
		output = os.Stderr
	}
	logger := log.New(output, "", log.LstdFlags)

	return p.RunWithClientAndLogger(client, logger)
}

// RunWithClientAndLogger runs a watch plan using an external client and
// log.Logger instance. Using this, the plan's Datacenter, Token and LogOutput
// fields are ignored and the passed client is expected to be configured as
// needed.
func (p *Plan) RunWithClientAndLogger(client *consulapi.Client,
	logger *log.Logger) error {

	p.client = client

	// Loop until we are canceled
	failures := 0
OUTER:
	for !p.shouldStop() {
		// Invoke the handler
		blockParamVal, result, err := p.Watcher(p)

		// Check if we should terminate since the function
		// could have blocked for a while
		if p.shouldStop() {
			break
		}

		// Handle an error in the watch function
		if err != nil {
			// Perform an exponential backoff
			failures++
			if blockParamVal == nil {
				p.lastParamVal = nil
			} else {
				p.lastParamVal = blockParamVal.Next(p.lastParamVal)
			}
			retry := retryInterval * time.Duration(failures*failures)
			if retry > maxBackoffTime {
				retry = maxBackoffTime
			}
			logger.Printf("[ERR] consul.watch: Watch (type: %s) errored: %v, retry in %v",
				p.Type, err, retry)
			select {
			case <-time.After(retry):
				continue OUTER
			case <-p.stopCh:
				return nil
			}
		}

		// Clear the failures
		failures = 0

		// If the index is unchanged do nothing
		if p.lastParamVal != nil && p.lastParamVal.Equal(blockParamVal) {
			continue
		}

		// Update the index, look for change
		oldParamVal := p.lastParamVal
		p.lastParamVal = blockParamVal.Next(oldParamVal)
		if oldParamVal != nil && reflect.DeepEqual(p.lastResult, result) {
			continue
		}

		// Handle the updated result
		p.lastResult = result
		// If a hybrid handler exists use that
		if p.HybridHandler != nil {
			p.HybridHandler(blockParamVal, result)
		} else if p.Handler != nil {
			idx, ok := blockParamVal.(WaitIndexVal)
			if !ok {
				logger.Printf("[ERR] consul.watch: Handler only supports index-based " +
					" watches but non index-based watch run. Skipping Handler.")
			}
			p.Handler(uint64(idx), result)
		}
	}
	return nil
}

// Stop is used to stop running the watch plan
func (p *Plan) Stop() {
	p.stopLock.Lock()
	defer p.stopLock.Unlock()
	if p.stop {
		return
	}
	p.stop = true
	if p.cancelFunc != nil {
		p.cancelFunc()
	}
	close(p.stopCh)
}

func (p *Plan) shouldStop() bool {
	select {
	case <-p.stopCh:
		return true
	default:
		return false
	}
}

func (p *Plan) setCancelFunc(cancel context.CancelFunc) {
	p.stopLock.Lock()
	defer p.stopLock.Unlock()
	if p.shouldStop() {
		// The watch is stopped and execute the new cancel func to stop watchFactory
		cancel()
		return
	}
	p.cancelFunc = cancel
}

func (p *Plan) IsStopped() bool {
	p.stopLock.Lock()
	defer p.stopLock.Unlock()
	return p.stop
}
