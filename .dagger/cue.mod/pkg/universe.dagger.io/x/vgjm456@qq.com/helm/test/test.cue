//Deprecated: in favor of universe.dagger.io/alpha package
package helm

import (
	"dagger.io/dagger"
	"universe.dagger.io/x/vgjm456@qq.com/helm"
)

dagger.#Plan & {
	client: {
		env: KUBECONFIG: string
		commands: kubeconfig: {
			name: "cat"
			args: ["\(env.KUBECONFIG)"]
			stdout: dagger.#Secret
		}
	}
	actions: test: {
		URL: helm.#Install & {
			name:       "test-pgsql"
			source:     "URL"
			URL:        "https://charts.bitnami.com/bitnami/postgresql-11.1.12.tgz"
			kubeconfig: client.commands.kubeconfig.stdout
		}
		repository: helm.#Install & {
			name:       "test-redis"
			source:     "repository"
			chart:      "redis"
			repoName:   "bitnami"
			repository: "https://charts.bitnami.com/bitnami"
			kubeconfig: client.commands.kubeconfig.stdout
		}
	}
}
