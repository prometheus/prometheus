// Copyright 2013 The Prometheus Authors
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
// limitations under the License.f

package rules

import (
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
)

var (
	customManager   *MysqlALert
	WorkerNum       = 10
	deafultInterval = "1m"
)

type MysqlALert struct {
	opts       *ManagerOptions
	mysqlGroup []*MysqlGroup
	logger     log.Logger
}

type MysqlGroup struct {
	index                int
	rules                map[int]Rule
	workerChan           chan *AlertRule
	opts                 *ManagerOptions
	interval             time.Duration
	seriesInPreviousEval []map[string]labels.Labels
	logger               log.Logger
	externalLabels       labels.Labels
}

func NewMysqlALert(o *ManagerOptions, cfg config.MysqlRuleConfig, m *MysqlALert, externalLabels labels.Labels) *MysqlALert {

	if m == nil {
		m = &MysqlALert{
			opts:       o,
			logger:     o.Logger,
			mysqlGroup: []*MysqlGroup{},
		}
		NewEngine(cfg)

		var k int
		for k = 0; k < WorkerNum; k++ {
			cg := newMysqlGroup(k, m.opts, deafultInterval, externalLabels)
			go cg.Run()
			m.mysqlGroup = append(m.mysqlGroup, cg)
		}
		customManager = m
		m.LoadRule()
	}
	return m
}

func newMysqlGroup(index int, opts *ManagerOptions, timeStr string, externalLabels labels.Labels) *MysqlGroup {
	n := &MysqlGroup{}
	n.index = index
	n.rules = map[int]Rule{}
	n.workerChan = make(chan *AlertRule, 1000)
	n.opts = opts
	n.interval, _ = time.ParseDuration(timeStr)
	n.externalLabels = externalLabels
	n.logger = log.With(opts.Logger, "group", fmt.Sprintf("worker-%d", n.index))
	return n
}

func (g *MysqlGroup) hash() uint64 {
	l := labels.New(
		labels.Label{"name", fmt.Sprintf("worker-%d", g.index)},
	)
	return l.Hash()
}

func (m *MysqlGroup) evalTimestamp() time.Time {
	var (
		offset = int64(m.hash() % uint64(m.interval))
		now    = time.Now().UnixNano()
		adjNow = now - offset
		base   = adjNow - (adjNow % int64(m.interval))
	)

	return time.Unix(0, base+offset)
}

func (m *MysqlGroup) RunRule() {

	evalTimestamp := m.evalTimestamp().Add(m.interval)

	app, err := m.opts.Appendable.Appender()
	if err != nil {
		fmt.Printf("get append fail:%v\n", err)
		return
	}

	for k, _ := range m.rules {
		rule := m.rules[k]
		vector, err := rule.EvalC(m.opts.Context, evalTimestamp, m.opts.QueryFunc, m.opts.ExternalURL)
		if err != nil {
			fmt.Printf("rule.Eval error:%v\n", err)
			continue
		}

		if ar, ok := rule.(*AlertingRule); ok {
			ar.sendAlertsC(m.opts.Context, evalTimestamp, m.opts.ResendDelay, m.interval, m.opts.NotifyFunc)
		}

		for _, s := range vector {
			app.Add(s.Metric, s.T, s.V)
		}

	}
}

func (m *MysqlGroup) AddRule(r *AlertRule) {
	m.workerChan <- r
}

func (m *MysqlGroup) fetchRule(r *AlertRule) *AlertingRule {
	expr, _ := promql.ParseExpr(r.Expr)

	tt, err := time.ParseDuration(r.Inter)
	if err != nil {
		tt, _ = time.ParseDuration("60m")
	}

	forDuration, err := time.ParseDuration(r.ForInter)
	if err != nil {
		tt, _ = time.ParseDuration("1m")
	}

	rule := NewAlertingRule(
		r.Name,
		expr,
		forDuration,
		tt,
		labels.FromMap(map[string]string{"severity": r.Severity}),
		labels.FromMap(map[string]string{"description": r.Descd}),
		m.externalLabels,
		true,
		log.With(m.logger, "alert", r.Name),
	)
	return rule
}

func (m *MysqlGroup) Run() {

	for {
		select {
		case <-time.After(time.Minute * time.Duration(1)):
			m.RunRule()
		case ruleModel := <-m.workerChan:
			ruleObject := m.fetchRule(ruleModel)
			_, ok := m.rules[ruleModel.Id]

			if ruleModel.Expr == "" {
				delete(m.rules, ruleModel.Id)
			} else {
				if ok {
					ruleObject.SetActive(m.rules[ruleModel.Id].GetActive(), ruleModel.Name)
				}
				m.rules[ruleModel.Id] = ruleObject
			}

		}
	}
}

func (m *MysqlALert) LoadRule() {
	l := []AlertRule{}
	Engine.Find(&l)
	for k, _ := range l {
		m.AddRule(&l[k])
	}
}

func (m *MysqlALert) AddRule(r *AlertRule) {

	h := fnv.New32a()
	ss := strconv.Itoa(r.Id)
	h.Write([]byte(ss))
	indexNum := int(h.Sum32()) % WorkerNum
	m.mysqlGroup[indexNum].AddRule(r)
}

type RuleJson struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Expr        string `json:"expr"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Interval    string `json:"sendInterval"`
	Index       int    `json:"index"`
	PageSize    int    `json:"pagesize"`
	For         string `json:"for"`
}

func ListAlert(rule *RuleJson) map[string]interface{} {
	l := []AlertRule{}

	dd := Engine.OrderBy("-id")
	cc := Engine.OrderBy("-id")

	if rule.Name != "" {
		dd = dd.Where("name like ?", "%"+rule.Name+"%")
		cc = cc.Where("name like ?", "%"+rule.Name+"%")
	}

	if rule.Expr != "" {
		dd = dd.Where("expr like ?", "%"+rule.Expr+"%")
		cc = cc.Where("expr like ?", "%"+rule.Expr+"%")
	}

	if rule.Description != "" {
		dd = dd.Where("descd like ?", "%"+rule.Description+"%")
		cc = cc.Where("descd like ?", "%"+rule.Description+"%")
	}

	if rule.PageSize != 0 {
		dd.Limit(rule.PageSize, (rule.Index-1)*rule.PageSize).Find(&l)
	} else {
		dd.Find(&l)
	}

	noListC := AlertRule{}
	counts, _ := cc.Count(&noListC)

	tmp := map[string]interface{}{}

	tmp["content"] = l
	tmp["totalLen"] = counts
	tmp["index"] = rule.Index

	return tmp
}

func DeleteAlert(rule *RuleJson) error {

	a := &AlertRule{}
	has, err := Engine.Where("id=?", rule.Id).Get(a)
	if has == false || err != nil {
		return errors.New("rule not exist")
	}

	a.Expr = ""
	_, err = Engine.Delete(&AlertRule{Id: a.Id})
	if err != nil {
		return err
	}

	NewHistory(a, "delete")

	customManager.AddRule(a)
	return nil
}

func ModifyAlert(rule *RuleJson) error {

	name := TrimeName(rule.Name)

	a := &AlertRule{}
	has, err := Engine.Where("id=?", rule.Id).Get(a)
	if has == false || err != nil {
		return errors.New("alert not exist")
	}
	a.Name = name
	Engine.Id(a.Id).Cols("name").Update(&AlertRule{Name: a.Name})

	_, err = promql.ParseExpr(rule.Expr)
	if err != nil {
		return err
	}

	a.Expr = rule.Expr
	Engine.Id(a.Id).Cols("expr").Update(&AlertRule{Expr: rule.Expr})

	serv := rule.Severity
	a.Severity = serv
	Engine.Id(a.Id).Cols("severity").Update(&AlertRule{Severity: rule.Severity})

	a.Descd = rule.Description
	Engine.Id(a.Id).Cols("descd").Update(&AlertRule{Descd: rule.Description})

	a.Inter = rule.Interval
	Engine.Id(a.Id).Cols("inter").Update(&AlertRule{Inter: a.Inter})

	a.ForInter = rule.For
	Engine.Id(a.Id).Cols("for_inter").Update(&AlertRule{ForInter: a.ForInter})

	NewHistory(a, "modify")

	customManager.AddRule(a)
	return nil
}

func AddRule(rule *RuleJson) error {

	a := &AlertRule{}

	name := TrimeName(rule.Name)
	if name == "" {
		return errors.New("name is null")
	}
	a.Name = name

	_, err := promql.ParseExpr(rule.Expr)
	if err != nil {
		return err
	}

	a.Expr = rule.Expr

	serv := rule.Severity
	a.Severity = serv
	a.Descd = rule.Description

	a.Inter = rule.Interval
	a.CreateTime = time.Now()
	a.Status = true
	a.ForInter = rule.For

	b := &AlertRule{}
	has, err := Engine.Where("name=? and expr=? and status=1", a.Name, a.Expr).Get(b)
	if err != nil {
		fmt.Printf("name and  expr duplicate:%v\n", err)
		return err
	}

	if has == true {
		return errors.New("name + expr exist")
	}

	_, err = Engine.Insert(a)
	if err != nil {
		return err
	}

	NewHistory(a, "add")

	has, err = Engine.Where("name=? and expr=? and status=1", a.Name, a.Expr).Get(b)
	if has == false || err != nil {
		fmt.Printf("add new but fetch fail:%v\n", err)
		return nil
	}

	customManager.AddRule(b)
	return nil
}

var (
	SpaceTrim, _ = regexp.Compile(`\s`)
)

func TrimeName(name string) string {
	c := SpaceTrim.ReplaceAllString(name, "")
	ret := []byte{}
	for k, _ := range c {
		a1, _ := regexp.Match(`[\d\w-\*]`, []byte{c[k]})
		if a1 == true {
			ret = append(ret, c[k])
		}
	}
	retS := string(ret)
	return retS
}
