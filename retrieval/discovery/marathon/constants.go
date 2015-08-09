package marathon

import (
	"github.com/prometheus/client_golang/model"
)

const (
	// AppLabel is used for the name of the app in Marathon.
	AppLabel model.LabelName = "__meta_marathon_app"

	// ImageLabel is the label that is used for the docker image running the service.
	ImageLabel model.LabelName = "__meta_marathon_image"

	// TaskLabel contains the mesos task name of the app instance.
	TaskLabel model.LabelName = "__meta_marathon_task"
)
