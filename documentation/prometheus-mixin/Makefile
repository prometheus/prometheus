JSONNET_FMT := jsonnetfmt -n 2 --max-blank-lines 2 --string-style s --comment-style s

help: ## Display available commands and their descriptions
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[38;2;230;82;44m<target>\033[0m\n\nTargets:\n"} /^[a-z0-9A-Z_-]+:.*?##/ { printf "  \033[38;2;230;82;44m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	
all: fmt prometheus_alerts.yaml dashboards_out lint ## Build all Prometheus mixin components (alerts, dashboards, format, lint)

fmt: ## Format all Jsonnet and libsonnet files
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		xargs -n 1 -- $(JSONNET_FMT) -i

prometheus_alerts.yaml: mixin.libsonnet config.libsonnet alerts.libsonnet ## Generate Prometheus alert rules YAML from Jsonnet
	jsonnet -S alerts.jsonnet > $@

dashboards_out: mixin.libsonnet config.libsonnet dashboards.libsonnet ## Generate Grafana dashboards JSON files from Jsonnet
	@mkdir -p dashboards_out
	jsonnet -J vendor -m dashboards_out dashboards.jsonnet

lint: prometheus_alerts.yaml ## Lint Jsonnet files and validate Prometheus alert rules
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		while read f; do \
			$(JSONNET_FMT) "$$f" | diff -u "$$f" -; \
		done

	promtool check rules prometheus_alerts.yaml

.PHONY: jb_install
jb_install: ## Install Jsonnet dependencies using jsonnet-bundler
	jb install

clean: ## Remove generated files (dashboards and alert rules)
	rm -rf dashboards_out prometheus_alerts.yaml