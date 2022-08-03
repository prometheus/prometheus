package goreleaser

import (
	"dagger.io/dagger"

	"universe.dagger.io/go"
)

// Release Go binaries using GoReleaser
#Release: {
	// Source code
	source: dagger.#FS

	// Custom GoReleaser image
	customImage: #Image

	// Don't publish or announce the release
	dryRun: bool | *false

	// Build a snapshot instead of a tag
	snapshot: bool | *false

	go.#Container & {
		name:     "goreleaser"
		"source": source
		image:    customImage.output

		entrypoint: [] // Support images that does not set goreleaser as the entrypoint
		command: {
			name: "goreleaser"

			flags: {
				if dryRun {
					"--skip-publish":  true
					"--skip-announce": true
				}

				if snapshot {
					"--snapshot": true
				}
			}
		}
	}
}
