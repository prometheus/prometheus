//Deprecated: in favor of universe.dagger.io/alpha package
package kubernetes

import (
	"dagger.io/dagger"

	"universe.dagger.io/x/tom.chauveau.pro@icloud.com/kubernetes"
)

dagger.#Plan & {
	client: filesystem: "./data/hello-kustomize": read: contents: dagger.#FS

	actions: test: {
		_source: client.filesystem."./data/hello-kustomize".read.contents

		// FIXME(TomChv): Depends on https://github.com/bitnami/bitnami-docker-kubectl/pull/43
		// url: kustomization: kubernetes.#Kustomize & {
		//  type: "url"
		//  url:  "https://github.com/kubernetes-sigs/kustomize.git/examples/helloWorld?ref=v1.0.6"
		// }

		inline: kubernetes.#Kustomize & {
			type:   "inline"
			source: _source
			kustomization: """
				apiVersion: kustomize.config.k8s.io/v1beta1
				kind: Kustomization
				resources:
				  - service.yaml
				namePrefix: inline-
				"""
			result: """
				apiVersion: v1
				kind: Service
				metadata:
				  labels:
				    dagger-test: dagger-test
				  name: inline-dagger-nginx-service
				spec:
				  ports:
				  - name: http
				    port: 80
				    protocol: TCP
				    targetPort: 80
				  selector:
				    app: dagger-nginx
				  type: ClusterIP\n
				"""
		}

		directory: kubernetes.#Kustomize & {
			type:          "directory"
			source:        _source
			kustomization: _source
			result: """
				apiVersion: v1
				kind: Service
				metadata:
				  labels:
				    dagger-test: dagger-test
				  name: kustom-dagger-nginx-service
				spec:
				  ports:
				  - name: http
				    port: 80
				    protocol: TCP
				    targetPort: 80
				  selector:
				    app: dagger-nginx
				  type: ClusterIP
				---
				apiVersion: apps/v1
				kind: Deployment
				metadata:
				  labels:
				    dagger-test: dagger-test
				  name: kustom-dagger-nginx
				spec:
				  replicas: 3
				  selector:
				    matchLabels:
				      app: dagger-nginx
				  template:
				    metadata:
				      labels:
				        app: dagger-nginx
				    spec:
				      containers:
				      - image: nginx:alpine
				        name: dagger-nginx
				        ports:
				        - containerPort: 80\n
				"""
		}
	}
}
