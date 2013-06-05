// Copyright 2013 Prometheus Team
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

package rules

import (
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage/metric"
	"log"
	"sync"
	"time"
)

type Result struct {
	Err     error // TODO propagate errors from rule evaluation.
	Samples model.Samples
}

type RuleManager interface {
	AddRulesFromConfig(config config.Config) error
	Run()
}

type ruleManager struct {
	// Protects the rules list.
	sync.Mutex
	rules []Rule

	results  chan *Result
	done     chan bool
	interval time.Duration
	storage  *metric.TieredStorage
}

func NewRuleManager(results chan *Result, interval time.Duration, storage *metric.TieredStorage) RuleManager {
	manager := &ruleManager{
		results:  results,
		rules:    []Rule{},
		done:     make(chan bool),
		interval: interval,
		storage:  storage,
	}
	return manager
}

func (m *ruleManager) Run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			start := time.Now()
			m.runIteration(m.results)
			evalDurations.Add(map[string]string{intervalKey: m.interval.String()}, float64(time.Since(start)/time.Millisecond))
		case <-m.done:
			log.Printf("RuleManager exiting...")
			break
		}
	}
}

func (m *ruleManager) Stop() {
	select {
	case m.done <- true:
	default:
	}
}

func (m *ruleManager) runIteration(results chan *Result) {
	now := time.Now()
	wg := sync.WaitGroup{}

	m.Lock()
	rules := make([]Rule, len(m.rules))
	copy(rules, m.rules)
	m.Unlock()

	for _, rule := range rules {
		wg.Add(1)
		// BUG(julius): Look at fixing thundering herd.
		go func(rule Rule) {
			defer wg.Done()
			vector, err := rule.Eval(now, m.storage)
			samples := make(model.Samples, len(vector))
			copy(samples, vector)
			m.results <- &Result{
				Samples: samples,
				Err:     err,
			}
		}(rule)
	}

	wg.Wait()
}

func (m *ruleManager) AddRulesFromConfig(config config.Config) error {
	for _, ruleFile := range config.Global.RuleFile {
		newRules, err := LoadRulesFromFile(ruleFile)
		if err != nil {
			return err
		}
		m.Lock()
		m.rules = append(m.rules, newRules...)
		m.Unlock()
	}
	return nil
}
