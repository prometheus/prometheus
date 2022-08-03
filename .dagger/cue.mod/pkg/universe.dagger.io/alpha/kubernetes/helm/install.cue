package helm

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

_#defaultNamespace: "default"

#Install: {
	// The image to use when running the action.
	// Must contain the helm binary. Defaults to alpine/helm
	image: *#Image | docker.#Image

	// Name of your release
	name:       string | *""
	kubeconfig: dagger.#Secret
	namespace:  *_#defaultNamespace | string
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
				helm upgrade --install --create-namespace --namespace $NAMESPACE $NAME $REPO_NAME/$CHART $GENERATE_NAME
				"""#
		}
	} | {
		source: "URL"
		URL:    string
		run: {
			env: "URL": URL
			_script: #"""
				helm upgrade --install --create-namespace --namespace $NAMESPACE $NAME $URL $GENERATE_NAME
				"""#
		}
	}

	run: docker.#Run & {
		input: image.output
		env: {
			NAME:          name
			GENERATE_NAME: _generateName
			NAMESPACE:     namespace
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
