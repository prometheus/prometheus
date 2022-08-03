package helm

import (
	"dagger.io/dagger"
	"universe.dagger.io/alpha/kubernetes/helm"
)

dagger.#Plan & {
	client: {
		filesystem: "./testdata": read: contents: dagger.#FS
		env: KUBECONFIG: string
		commands: kubeconfig: {
			name: "cat"
			args: ["\(env.KUBECONFIG)"]
			stdout: dagger.#Secret
		}
	}
	actions: test: {
		install: {
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
		upgrade: helm.#Upgrade & {
			kubeconfig:    client.commands.kubeconfig.stdout
			workspace:     client.filesystem."./testdata".read.contents
			name:          "redis"
			repo:          "https://charts.bitnami.com/bitnami"
			chart:         "redis"
			version:       "17.0.1"
			namespace:     "dagger-helm-upgrade-test"
			atomic:        true
			install:       true
			cleanupOnFail: true
			debug:         true
			force:         true
			wait:          true
			timeout:       "2m"
			flags: ["--skip-crds", "--description='Dagger Test Run'"]
			values: ["values.base.yaml", "values.staging.yaml"]
			set: #"""
				architecture=standalone
				auth.enabled=false
				commonLabels.dagger\.io/set=val
				"""#
			setString: #"""
				master.podAnnotations.n=1
				master.podLabels.n=2
				"""#
		}
	}
}
