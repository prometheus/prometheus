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

		url: kustomization: kubernetes.#Kustomize & {
			type: "url"
			url:  "https://github.com/kubernetes-sigs/kustomize.git/examples/helloWorld?ref=v1.0.6"
			result: """
				apiVersion: v1
				data:
				  altGreeting: Good Morning!
				  enableRisky: "false"
				kind: ConfigMap
				metadata:
				  labels:
				    app: hello
				  name: the-map
				---
				apiVersion: v1
				kind: Service
				metadata:
				  labels:
				    app: hello
				  name: the-service
				spec:
				  ports:
				  - port: 8666
				    protocol: TCP
				    targetPort: 8080
				  selector:
				    app: hello
				    deployment: hello
				  type: LoadBalancer
				---
				apiVersion: apps/v1
				kind: Deployment
				metadata:
				  labels:
				    app: hello
				  name: the-deployment
				spec:
				  replicas: 3
				  selector:
				    matchLabels:
				      app: hello
				  template:
				    metadata:
				      labels:
				        app: hello
				        deployment: hello
				    spec:
				      containers:
				      - command:
				        - /hello
				        - --port=8080
				        - --enableRiskyFeature=$(ENABLE_RISKY)
				        env:
				        - name: ALT_GREETING
				          valueFrom:
				            configMapKeyRef:
				              key: altGreeting
				              name: the-map
				        - name: ENABLE_RISKY
				          valueFrom:
				            configMapKeyRef:
				              key: enableRisky
				              name: the-map
				        image: monopole/hello:1
				        name: the-container
				        ports:
				        - containerPort: 8080\n
				"""
		}

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
