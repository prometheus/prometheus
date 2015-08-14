package marathon

import (
	"fmt"

	clientmodel "github.com/prometheus/client_golang/model"

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
		appName = clientmodel.LabelValue(app.ID)
		image   = clientmodel.LabelValue(app.Container.Docker.Image)
	)
	tg := &config.TargetGroup{
		Targets: targets,
		Labels: clientmodel.LabelSet{
			appLabel:   appName,
			imageLabel: image,
		},
		Source: app.ID,
	}

	for ln, lv := range app.Labels {
		ln = appLabelPrefix + ln
		tg.Labels[clientmodel.LabelName(ln)] = clientmodel.LabelValue(lv)
	}

	return tg
}

func targetsForApp(app *App) []clientmodel.LabelSet {
	targets := make([]clientmodel.LabelSet, 0, len(app.Tasks))
	for _, t := range app.Tasks {
		target := targetForTask(&t)
		targets = append(targets, clientmodel.LabelSet{
			clientmodel.AddressLabel: clientmodel.LabelValue(target),
			taskLabel:                clientmodel.LabelValue(t.ID),
		})
	}
	return targets
}

func targetForTask(task *Task) string {
	return fmt.Sprintf("%s:%d", task.Host, task.Ports[0])
}
