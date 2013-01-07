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
	"github.com/matttproud/prometheus/config"
	"github.com/matttproud/prometheus/rules/ast"
	"log"
	"time"
)

type Result struct {
	Err     error // TODO propagate errors from rule evaluation.
	Samples ast.Vector
}

type RuleManager interface {
	AddRulesFromConfig(config *config.Config) error
}

type ruleManager struct {
	rules    []*Rule
	results  chan *Result
	done     chan bool
	interval time.Duration
}

func NewRuleManager(results chan *Result, interval time.Duration) RuleManager {
	manager := &ruleManager{
		results:  results,
		rules:    []*Rule{},
		interval: interval,
	}
	go manager.run(results)
	return manager
}

func (m *ruleManager) run(results chan *Result) {
	ticker := time.Tick(m.interval)

	for {
		select {
		case <-ticker:
			m.runIteration(results)
		case <-m.done:
			log.Printf("RuleManager exiting...")
			break
		}
	}
}

func (m *ruleManager) Stop() {
	m.done <- true
}

func (m *ruleManager) runIteration(results chan *Result) {
	now := time.Now()
	for _, rule := range m.rules {
		go func() {
			vector := rule.Eval(&now)
			m.results <- &Result{
				Samples: vector,
			}
		}()
	}
}

func (m *ruleManager) AddRulesFromConfig(config *config.Config) error {
	for _, ruleFile := range config.Global.RuleFiles {
		newRules, err := LoadFromFile(ruleFile)
		if err != nil {
			return err
		}
		m.rules = append(m.rules, newRules...)
	}
	return nil
}
