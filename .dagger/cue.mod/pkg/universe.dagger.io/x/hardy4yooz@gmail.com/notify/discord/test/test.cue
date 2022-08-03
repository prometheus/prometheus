//Deprecated: in favor of universe.dagger.io/alpha package
package discord

import (
	"dagger.io/dagger"

	"universe.dagger.io/x/hardy4yooz@gmail.com/notify/discord"
)

dagger.#Plan & {
	client: env: DISCORD_WEBHOOK_URL: dagger.#Secret

	actions: test: {
		// Simple parameters for test usage
		simple: discord.#Send & {
			webhookUrl: client.env.DISCORD_WEBHOOK_URL
			parameters: {
				username:   "Dagger"
				avatar_url: "https://cdn.discordapp.com/icons/707636530424053791/dbe8b1c82fc49086c06249e4e28750d2.webp?size=240"
				content:    "DAGGER - A PORTABLE DEVKIT FOR CI/CD PIPELINES."
			}
		}

		// With embed objects
		embed: discord.#Send & {
			webhookUrl: client.env.DISCORD_WEBHOOK_URL
			parameters: {
				username:   "Dagger BOT"
				avatar_url: "https://cdn.discordapp.com/icons/707636530424053791/dbe8b1c82fc49086c06249e4e28750d2.webp?size=240"
				content:    "New blog released!"
				embeds: [
					{
						author: {
							name:     "Dagger team"
							url:      "https://dagger.io/"
							icon_url: "https://cdn.discordapp.com/icons/707636530424053791/dbe8b1c82fc49086c06249e4e28750d2.webp?size=240"
						}
						title:       "What is Dagger?"
						url:         "https://docs.dagger.io/1235/what"
						description: "Using Dagger, software teams can develop powerful CICD pipelines with minimal effort, then run them anywhere."
						color:       131226
						fields: [
							{
								name:  "Benefits"
								value: "More text"
							},
							{
								name:  "How does it work?"
								value: "More text"
							},
						]
						thumbnail: url: "https://github.githubassets.com/images/modules/logos_page/Octocat.png"
						image: {
							url:    "https://cdn.discordapp.com/icons/707636530424053791/dbe8b1c82fc49086c06249e4e28750d2.webp?size=240"
							height: 25
							width:  25
						}
						footer: {
							text:     "Join us ..."
							icon_url: "https://github.githubassets.com/images/modules/logos_page/GitHub-Mark.png"
						}
					},
				]
			}
		}
	}
}
