package composer

import (
	"dagger.io/dagger"
)

// Composer command
#Run: {
	// Source code
	source: dagger.#FS

	env: [string]: string

	// Arguments for composer
	args: [...string]

	container: #Container & {
		"source": source
		"env":    env
		command: {
			name:   "composer"
			"args": args
		}
		export: {
			directories: "/output/vendor": _
			files: {
				"/output/composer.json": _
				"/output/composer.lock": _
			}
		}
	}

	// Export packages and composer files
	output: {
		vendor: container.export.directories."/output/vendor"
		json:   container.export.files."/output/composer.json"
		lock:   container.export.files."/output/composer.lock"
	}
}
