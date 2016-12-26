package types

// ServiceInstance represents an ECS service instance
type ServiceInstance struct {
	Addr      string
	Cluster   string
	Service   string
	Container string
	Tags      map[string]string
	Labels    map[string]string
}
