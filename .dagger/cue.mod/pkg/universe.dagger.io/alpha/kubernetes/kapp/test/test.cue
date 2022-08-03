package kapp

import (
	"dagger.io/dagger"
	"universe.dagger.io/bash"
)

#AssertDep: {
	fs:         dagger.#FS
	kubeConfig: dagger.#Secret

	_image: #Image & {
		imgFs: fs
	}
	run: bash.#Run & {
		input: _image.output
		script: contents:
			#"""
				    test "$(kapp ls  --column name | grep -c dtest)" = "1"
				"""#
		mounts: "/root/.kube/config": {
			dest:     "/root/.kube/config"
			type:     "secret"
			contents: kubeConfig
		}
	}
}

dagger.#Plan & {
	actions: test: {
		deploy: #Deploy & {
			app:        "dtest"
			fs:         client.filesystem."./".read.contents
			kubeConfig: client.commands.kc.stdout
			file:       "./kubesrc.yaml"
		}
		verify: #AssertDep & {
			fs:         client.filesystem."./".read.contents
			kubeConfig: client.commands.kc.stdout
		}
		ls: #List & {
			fs:         client.filesystem."./".read.contents
			kubeConfig: client.commands.kc.stdout
			namespace:  "default"
		}
		inspect: #Inspect & {
			app:        "dtest"
			fs:         client.filesystem."./".read.contents
			kubeConfig: client.commands.kc.stdout
		}
		delete: #Delete & {
			app:        "dtest"
			fs:         client.filesystem."./".read.contents
			kubeConfig: client.commands.kc.stdout
		}
	}

	client: {
		commands: kc: {
			name: "kubectl"
			args: ["config", "view", "--raw"]
			stdout: dagger.#Secret
		}
		filesystem: "./": read: {
			contents: dagger.#FS
			include: ["kubesrc.yaml"]
		}
	}
}
