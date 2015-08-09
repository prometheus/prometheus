package marathon

// Task describes one instance of a service running on Marathon.
type Task struct {
	ID    string   `json:"id"`
	Host  string   `json:"host"`
	Ports []uint32 `json:"ports"`
}

// DockerContainer describes a container which uses the docker runtime.
type DockerContainer struct {
	Image string `json:"image"`
}

// Container describes the runtime an app in running in.
type Container struct {
	Docker DockerContainer `json:"docker"`
}

// App describes a service running on Marathon.
type App struct {
	ID           string            `json:"id"`
	Tasks        []Task            `json:"tasks"`
	RunningTasks int               `json:"tasksRunning"`
	Labels       map[string]string `json:"labels"`
	Container    Container         `json:"container"`
}

// AppList is a list of Marathon apps.
type AppList struct {
	Apps []App `json:"apps"`
}
