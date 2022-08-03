//Deprecated: in favor of universe.dagger.io/alpha package
package discord

import (
	"encoding/json"

	"dagger.io/dagger"

	"universe.dagger.io/alpine"
	"universe.dagger.io/bash"
)

// Send message to discord channel
#Send: {
	// Discord Webhook URL
	webhookUrl: dagger.#Secret

	// Discord json parameters
	parameters: #Parameter

	_image: alpine.#Build & {
		packages: {
			bash: {}
			curl: {}
		}
	}

	bash.#Run & {
		input: _image.output
		env: {
			DISCORD_WEBHOOK_URL: webhookUrl
			DATA:                json.Marshal(parameters)
		}
		script: contents: "curl -X POST ${DISCORD_WEBHOOK_URL} -H 'Content-Type: application/json' -d \"${DATA}\""
	}
}
