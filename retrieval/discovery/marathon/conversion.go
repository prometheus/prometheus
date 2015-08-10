package marathon

import (
	"fmt"
	"strings"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
)

// AppsToTargetGroups takes an array of Marathon apps and converts them into target groups.
func AppsToTargetGroups(apps *AppList) map[string]*config.TargetGroup {
	tgroups := map[string]*config.TargetGroup{}
	for _, a := range apps.Apps {
		if isValidApp(&a) {
			group := createTargetGroup(&a)
			tgroups[group.Source] = group
		}
	}
	return tgroups
}

func createTargetGroup(app *App) *config.TargetGroup {
	var (
		targets = targetsForApp(app)
		source  = targetGroupName(app)
		appName = clientmodel.LabelValue(sanitizeName(app.ID))
		image   = clientmodel.LabelValue(imageName(app))
	)
	return &config.TargetGroup{
		Targets: targets,
		Labels: clientmodel.LabelSet{
			AppLabel:   appName,
			ImageLabel: image,
		},
		Source: source,
	}
}

func targetsForApp(app *App) []clientmodel.LabelSet {
	targets := make([]clientmodel.LabelSet, 0, len(app.Tasks))
	for _, t := range app.Tasks {
		target := targetForTask(&t)
		targets = append(targets, clientmodel.LabelSet{
			clientmodel.AddressLabel: clientmodel.LabelValue(target),
			TaskLabel:                clientmodel.LabelValue(sanitizeName(t.ID)),
		})
	}
	return targets
}

func imageName(app *App) string {
	return app.Container.Docker.Image
}

func targetForTask(task *Task) string {
	return fmt.Sprintf("%s:%d", task.Host, task.Ports[0])
}

func isValidApp(app *App) bool {
	if app.RunningTasks > 0 {
		_, ok := app.Labels["prometheus"]
		return ok
	}
	return false
}

func targetGroupName(app *App) string {
	return sanitizeName(app.ID)
}

func sanitizeName(id string) string {
	trimID := strings.TrimLeft(id, " -/.")
	return strings.Replace(trimID, "/", "-", -1)
}
