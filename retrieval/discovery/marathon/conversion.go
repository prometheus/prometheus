// Copyright 2015 The Prometheus Authors
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

package marathon

import (
	"fmt"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

// AppsToTargetGroups takes an array of Marathon apps and converts them into target groups.
func AppsToTargetGroups(apps *AppList) map[string]*config.TargetGroup {
	tgroups := map[string]*config.TargetGroup{}
	for _, a := range apps.Apps {
		group := createTargetGroup(&a)
		tgroups[group.Source] = group
	}
	return tgroups
}

func createTargetGroup(app *App) *config.TargetGroup {
	var (
		targets = targetsForApp(app)
		appName = model.LabelValue(app.ID)
		image   = model.LabelValue(app.Container.Docker.Image)
	)
	tg := &config.TargetGroup{
		Targets: targets,
		Labels: model.LabelSet{
			appLabel:   appName,
			imageLabel: image,
		},
		Source: app.ID,
	}

	for ln, lv := range app.Labels {
		ln = appLabelPrefix + ln
		tg.Labels[model.LabelName(ln)] = model.LabelValue(lv)
	}

	return tg
}

func targetsForApp(app *App) []model.LabelSet {
	targets := make([]model.LabelSet, 0, len(app.Tasks))
	for _, t := range app.Tasks {
		target := targetForTask(&t)
		targets = append(targets, model.LabelSet{
			model.AddressLabel: model.LabelValue(target),
			taskLabel:          model.LabelValue(t.ID),
		})
	}
	return targets
}

func targetForTask(task *Task) string {
	return fmt.Sprintf("%s:%d", task.Host, task.Ports[0])
}
