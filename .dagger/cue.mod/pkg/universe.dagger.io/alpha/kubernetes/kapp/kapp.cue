package kapp

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

//This is an experimental feature.
//Currently works with 'kind' cluster
//Can be imported as universe.dagger.io/x/carvel.dev/kapp

_#entryPoint: {
	deployScript: #"""
		    if [ -d /source ] || [ -f /source ]; then
		        kapp deploy -n "$KAPP_NAMESPACE" -a "$APP_NAME" -f /source -y
		        exit $?
		    fi
		    if [ -n "$DEPLOYMENT_URL" ]; then
		        kapp deploy -n "$KAPP_NAMESPACE" -a "$APP_NAME" -f "$DEPLOYMENT_URL" -y
		        exit $?
		    fi
		"""#

	deleteScript: #"""
		    kapp delete -n "$KAPP_NAMESPACE" -a "$APP_NAME"  -y
		    exit $?
		"""#

	listScript: #"""
		    kapp ls -n "$KAPP_NAMESPACE"
		    exit $?
		"""#

	inspectScript: #"""
		    kapp inspect -n "$KAPP_NAMESPACE" -a "$APP_NAME"  -y
		    exit $?
		"""#
}

// Install Kapp cli
#Image: {

	imgFs: dagger.#FS

	source: string | *null

	build: docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "ghcr.io/vmware-tanzu/carvel-docker-image"
			},
			if source != null {
				docker.#Copy & {
					dest:     "/source"
					"source": source
					contents: imgFs
				}
			},
		]
	}

	output: build.output
}

// kapp deploy command
#Deploy: {

	//kapp app name
	app: string

	// Kubernetes Namespace to deploy to
	namespace: string | *"default"

	//kube config
	kubeConfig: dagger.#Secret

	//link to the manifest
	url: string | *null

	//file name with present in the client fs
	file: string | *null

	//client fs
	fs: dagger.#FS

	_image: #Image & {
		imgFs:  fs
		source: file
	}

	container: docker.#Run & {
		input:  _image.output
		always: true
		entrypoint: ["/bin/sh"]
		command: {
			name: "-c"
			args: [_#entryPoint.deployScript]
		}
		env: {
			APP_NAME:       app
			KAPP_NAMESPACE: namespace
			if url != null {
				DEPLOYMENT_URL: url
			}
		}
		mounts: "/root/.kube/config": {
			dest:     "/root/.kube/config"
			type:     "secret"
			contents: kubeConfig
		}
	}
}

// kapp delete command
#Delete: {

	// Kubernetes Namespace in which the app to delete is present
	namespace: string | *"default"

	kubeConfig: dagger.#Secret

	//kapp app name
	app: string

	url: string | *null

	fs: dagger.#FS

	_image: #Image & {
		imgFs: fs
	}

	container: docker.#Run & {
		input:  _image.output
		always: true
		entrypoint: ["/bin/sh"]
		command: {
			name: "-c"
			args: [_#entryPoint.deleteScript]
		}
		env: {
			APP_NAME:       app
			KAPP_NAMESPACE: namespace
		}
		mounts: "/root/.kube/config": {
			dest:     "/root/.kube/config"
			type:     "secret"
			contents: kubeConfig
		}
	}
}

// kapp list command
#List: {

	// Kubernetes Namespace
	namespace: string | *"default"

	kubeConfig: dagger.#Secret

	url: string | *null

	fs: dagger.#FS

	_image: #Image & {
		imgFs: fs
	}

	container: docker.#Run & {
		input:  _image.output
		always: true
		entrypoint: ["/bin/sh"]
		command: {
			name: "-c"
			args: [_#entryPoint.listScript]
		}
		env: KAPP_NAMESPACE: namespace
		mounts: "/root/.kube/config": {
			dest:     "/root/.kube/config"
			type:     "secret"
			contents: kubeConfig
		}
	}
}

// kapp Inspect command
#Inspect: {

	// Kubernetes Namespace
	namespace: string | *"default"

	kubeConfig: dagger.#Secret

	//kapp app name
	app: string

	url: string | *null

	fs: dagger.#FS

	_image: #Image & {
		imgFs: fs
	}

	container: docker.#Run & {
		input:  _image.output
		always: true
		entrypoint: ["/bin/sh"]
		command: {
			name: "-c"
			args: [_#entryPoint.inspectScript]
		}
		env: {
			APP_NAME:       app
			KAPP_NAMESPACE: namespace
		}
		mounts: "/root/.kube/config": {
			dest:     "/root/.kube/config"
			type:     "secret"
			contents: kubeConfig
		}
	}
}
