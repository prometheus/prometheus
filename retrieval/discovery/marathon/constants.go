package marathon

import (
	clientmodel "github.com/prometheus/client_golang/model"
)

const (
	// metaLabelPrefix is the meta prefix used for all meta labels in this discovery.
	metaLabelPrefix = clientmodel.MetaLabelPrefix + "marathon_"
	// appLabelPrefix is the prefix for the application labels.
	appLabelPrefix = metaLabelPrefix + "app_label_"

	// appLabel is used for the name of the app in Marathon.
	appLabel clientmodel.LabelName = metaLabelPrefix + "app"
	// imageLabel is the label that is used for the docker image running the service.
	imageLabel clientmodel.LabelName = metaLabelPrefix + "image"
	// taskLabel contains the mesos task name of the app instance.
	taskLabel clientmodel.LabelName = metaLabelPrefix + "task"
)
