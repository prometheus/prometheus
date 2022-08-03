// Run and deploy the Nginx web server
// https://nginx.org
package nginx

import (
	"universe.dagger.io/docker"
)

// Build a nginx container image
// FIXME: bootstrapping by wrapping "docker pull nginx"
//    Possible ways to improve:
//    1. "docker build" the docker hub image ourselves: https://github.com/nginxinc/docker-nginx
//    2. Reimplement same docker build in pure Cue (no more Dockerfile)
// FIXME: build from source or package distro, instead of docker pull
#Build: {
	output: docker.#Image & _pull.image

	_pull: docker.#Pull
	*{
		flavor: "alpine"
		_pull: source: "index.docker.io/nginx:stable-alpine"
	} | {
		flavor: "debian"
		_pull: source: "index.docker.io/nginx:stable"
	}
}
