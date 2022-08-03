//Deprecated: in favor of universe.dagger.io/alpha package
package helm

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

#Install: {
	// Name of your release
	name:       string | *""
	kubeconfig: dagger.#Secret
	source:     *"repository" | "URL"
	{
		source:     "repository"
		chart:      string
		repoName:   string
		repository: string
		run: {
			env: {
				CHART:      chart
				REPO_NAME:  repoName
				REPOSITORY: repository
			}
			_script: #"""
				helm repo add $REPO_NAME $REPOSITORY
				helm repo update
				helm install $NAME $REPO_NAME/$CHART $GENERATE_NAME
				"""#
		}
	} | {
		source: "URL"
		URL:    string
		run: {
			env: "URL": URL
			_script: #"""
				helm install $NAME $URL $GENERATE_NAME
				"""#
		}
	}

	_base: #Image
	run:   docker.#Run & {
		input: _base.output
		env: {
			NAME:          name
			GENERATE_NAME: _generateName
		}
		mounts: "/root/.kube/config": {
			dest:     "/root/.kube/config"
			type:     "secret"
			contents: kubeconfig
		}
		entrypoint: ["/bin/sh"]
		command: {
			name: "-c"
			args: [run._script]
		}
	}

	_generateName: string | *""
	if name == "" {
		_generateName: "--generate-name"
	}
}
