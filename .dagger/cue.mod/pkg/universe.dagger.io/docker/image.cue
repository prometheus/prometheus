package docker

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
)

// A container image
#Image: {
	// Root filesystem of the image.
	rootfs: dagger.#FS

	// Image config
	config: core.#ImageConfig
}

// An empty container image (same as `FROM scratch` in a Dockerfile)
#Scratch: #Image & {
	rootfs: dagger.#Scratch
	config: {}
}

// A ref is an address for a remote container image
// Examples:
//   - "index.docker.io/dagger"
//   - "dagger"
//   - "index.docker.io/dagger:latest"
//   - "index.docker.io/dagger:latest@sha256:a89cb097693dd354de598d279c304a1c73ee550fbfff6d9ee515568e0c749cfe"
// FIXME: add formatting constraints
#Ref: core.#Ref
