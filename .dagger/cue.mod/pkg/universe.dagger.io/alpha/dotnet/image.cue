package dotnet

import (
	"universe.dagger.io/docker"
)

// .NET image default version
_#DefaultVersion: "6.0"

// Pull a dotnet base image
#Image: {
	version: *_#DefaultVersion | string
	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "mcr.microsoft.com/dotnet/sdk:\(version)-alpine"
			},
		]
	}
}
