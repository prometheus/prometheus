package types

// ServiceInstance represents an ECS service instance
type ServiceInstance struct {
	// Addr is the address of the target instance
	Addr string
	// cluster is the cluster name where the target instance resides
	Cluster string
	// Service is the service that owns the target instance
	Service string
	// Contaner is the container name of th target instance
	Container string
	// Tags ar the EC2 instance tags of the target instance
	Tags map[string]string
	// Labels are the Docker container instance labels of the target instance
	Labels map[string]string
	// Image is the docker image that has been used to run the container
	Image string
}
