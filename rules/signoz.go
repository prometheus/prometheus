package rules

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
)

func (m *Manager) EditGroup(interval time.Duration, rule string, groupName string) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.restored = true

	var groups map[string]*Group
	var errs []error

	groups, errs = m.LoadGroup(interval, rule, groupName)

	if errs != nil {
		for _, e := range errs {
			level.Error(m.logger).Log("msg", "loading groups failed", "err", e)
		}
		return errors.New("error loading rules, previous rule set restored")
	}

	var wg sync.WaitGroup

	for _, newg := range groups {
		wg.Add(1)

		// If there is an old group with the same identifier, stop it and wait for
		// it to finish the current iteration. Then copy it into the new group.
		gn := GroupKey(newg.name, newg.file)
		oldg, ok := m.groups[gn]
		if !ok {
			return errors.New("rule not found")
		}

		delete(m.groups, gn)

		go func(newg *Group) {
			if ok {
				oldg.stop()
				newg.CopyState(oldg)
			}
			go func() {
				// Wait with starting evaluation until the rule manager
				// is told to run. This is necessary to avoid running
				// queries against a bootstrapping storage.
				<-m.block
				newg.run(m.opts.Context)
			}()
			wg.Done()
		}(newg)

		m.groups[gn] = newg

	}

	// // Stop remaining old groups.
	// for _, oldg := range m.groups {
	// 	oldg.stop()
	// }

	wg.Wait()

	return nil
}

func (m *Manager) UpdateGroupWithAction(interval time.Duration, rule string, groupName string, action string) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.restored = true

	var groups map[string]*Group
	var errs []error

	if action == "add" {
		groups, errs = m.LoadAddedGroups(interval, rule, groupName)
	} else if action == "delete" {
		groups, errs = m.LoadDeletedGroups(interval, groupName)
	}

	if errs != nil {
		for _, e := range errs {
			level.Error(m.logger).Log("msg", "loading groups failed", "err", e)
		}
		return errors.New("error loading rules, previous rule set restored")
	}

	var wg sync.WaitGroup

	for _, newg := range groups {
		wg.Add(1)

		// If there is an old group with the same identifier, stop it and wait for
		// it to finish the current iteration. Then copy it into the new group.
		gn := GroupKey(newg.name, newg.file)
		oldg, ok := m.groups[gn]
		delete(m.groups, gn)

		go func(newg *Group) {
			if ok {
				oldg.stop()
				newg.CopyState(oldg)
			}
			go func() {
				// Wait with starting evaluation until the rule manager
				// is told to run. This is necessary to avoid running
				// queries against a bootstrapping storage.
				<-m.block
				newg.run(m.opts.Context)
			}()
			wg.Done()
		}(newg)
	}

	// Stop remaining old groups.
	for _, oldg := range m.groups {
		oldg.stop()
	}

	wg.Wait()
	m.groups = groups

	return nil
}
func (m *Manager) DeleteGroup(interval time.Duration, rule string, groupName string) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.restored = true

	filename := "webAppEditor"

	gn := GroupKey(groupName, filename)
	oldg, ok := m.groups[gn]

	var wg sync.WaitGroup
	wg.Add(1)
	go func(newg *Group) {
		if ok {
			oldg.stop()
			delete(m.groups, gn)
		}
		defer wg.Done()
	}(oldg)

	wg.Wait()

	return nil
}

func (m *Manager) AddGroup(interval time.Duration, rule string, groupName string) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.restored = true

	var groups map[string]*Group
	var errs []error

	groups, errs = m.LoadGroup(interval, rule, groupName)

	if errs != nil {
		for _, e := range errs {
			level.Error(m.logger).Log("msg", "loading groups failed", "err", e)
		}
		return errors.New("error loading rules, previous rule set restored")
	}

	var wg sync.WaitGroup

	for _, newg := range groups {
		wg.Add(1)

		// If there is an old group with the same identifier, stop it and wait for
		// it to finish the current iteration. Then copy it into the new group.
		gn := GroupKey(newg.name, newg.file)
		oldg, ok := m.groups[gn]
		delete(m.groups, gn)

		go func(newg *Group) {
			if ok {
				oldg.stop()
				newg.CopyState(oldg)
			}
			go func() {
				// Wait with starting evaluation until the rule manager
				// is told to run. This is necessary to avoid running
				// queries against a bootstrapping storage.
				<-m.block
				newg.run(m.opts.Context)
			}()
			wg.Done()
		}(newg)

		if !ok {
			m.groups[gn] = newg
		}
	}

	wg.Wait()

	return nil
}

// LoadGroups reads groups from a list of files.
func (m *Manager) LoadGroup(interval time.Duration, rule string, groupName string) (map[string]*Group, []error) {
	groups := make(map[string]*Group)

	shouldRestore := !m.restored

	filename := "webAppEditor"

	rg, errs := rulefmt.Parse([]byte(rule))

	if errs != nil {
		return nil, errs
	}

	rules := make([]Rule, 0)
	for _, g := range rg.Groups {
		for _, r := range g.Rules {
			expr, err := m.opts.GroupLoader.Parse(r.Expr.Value)
			if err != nil {
				return nil, []error{err}
			}

			if r.Alert.Value != "" {
				rules = append(rules, NewAlertingRule(
					r.Alert.Value,
					expr,
					time.Duration(r.For),
					labels.FromMap(r.Labels),
					labels.FromMap(r.Annotations),
					labels.EmptyLabels(),
					"",
					m.restored,
					log.With(m.logger, "alert", r.Alert.Value),
				))
				continue
			}
			rules = append(rules, NewRecordingRule(
				r.Record.Value,
				expr,
				labels.FromMap(r.Labels),
			))
		}
		itv := interval
		if g.Interval != 0 {
			itv = time.Duration(g.Interval)
		}
		opts := GroupOptions{
			Name:          g.Name,
			File:          filename,
			Interval:      itv,
			Limit:         g.Limit,
			Rules:         rules,
			ShouldRestore: shouldRestore,
			Opts:          m.opts,
			done:          m.done,
		}
		groups[GroupKey(g.Name, filename)] = NewGroup(opts)
	}

	return groups, nil
}

// LoadGroups reads groups from a list of files.
func (m *Manager) LoadAddedGroups(interval time.Duration, rule string, groupName string) (map[string]*Group, []error) {
	groups := make(map[string]*Group)

	shouldRestore := !m.restored

	filename := "webAppEditor"

	rg, errs := rulefmt.Parse([]byte(rule))

	if errs != nil {
		return nil, errs
	}

	customGroups := m.RuleGroupsWithoutLock()
	for _, group := range customGroups {

		groups[GroupKey(group.Name(), group.File())] = group
	}

	rules := make([]Rule, 0)
	for _, g := range rg.Groups {
		for _, r := range g.Rules {
			expr, err := m.opts.GroupLoader.Parse(r.Expr.Value)
			if err != nil {
				return nil, []error{err}
			}

			if r.Alert.Value != "" {
				rules = append(rules, NewAlertingRule(
					r.Alert.Value,
					expr,
					time.Duration(r.For),
					labels.FromMap(r.Labels),
					labels.FromMap(r.Annotations),
					labels.EmptyLabels(),
					"",
					m.restored,
					log.With(m.logger, "alert", r.Alert.Value),
				))
				continue
			}
			rules = append(rules, NewRecordingRule(
				r.Record.Value,
				expr,
				labels.FromMap(r.Labels),
			))
		}
		itv := interval
		if g.Interval != 0 {
			itv = time.Duration(g.Interval)
		}
		opts := GroupOptions{
			Name:          g.Name,
			File:          filename,
			Interval:      itv,
			Limit:         g.Limit,
			Rules:         rules,
			ShouldRestore: shouldRestore,
			Opts:          m.opts,
			done:          m.done,
		}
		groups[GroupKey(g.Name, filename)] = NewGroup(opts)
	}

	return groups, nil
}

// LoadGroups reads groups from a list of files.
func (m *Manager) LoadDeletedGroups(interval time.Duration, groupName string) (map[string]*Group, []error) {
	groups := make(map[string]*Group)

	filename := "webAppEditor"

	customGroups := m.RuleGroupsWithoutLock()
	for _, group := range customGroups {

		groups[GroupKey(group.Name(), group.File())] = group
	}

	delete(groups, GroupKey(groupName, filename))

	return groups, nil
}

// RuleGroups returns the list of manager's rule groups.
func (m *Manager) RuleGroupsWithoutLock() []*Group {

	rgs := make([]*Group, 0, len(m.groups))
	for _, g := range m.groups {
		rgs = append(rgs, g)
	}

	sort.Slice(rgs, func(i, j int) bool {
		return rgs[i].file < rgs[j].file && rgs[i].name < rgs[j].name
	})

	return rgs
}
