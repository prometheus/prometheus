JSONNET_FMT := jsonnetfmt -n 2 --max-blank-lines 2 --string-style s --comment-style s

all: fmt prometheus_alerts.yaml dashboards_out lint

fmt:
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		xargs -n 1 -- $(JSONNET_FMT) -i

prometheus_alerts.yaml: mixin.libsonnet config.libsonnet alerts.libsonnet
	jsonnet -S alerts.jsonnet > $@

dashboards_out: mixin.libsonnet config.libsonnet dashboards.libsonnet
	@mkdir -p dashboards_out
	jsonnet -J vendor -m dashboards_out dashboards.jsonnet

lint: prometheus_alerts.yaml
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		while read f; do \
			$(JSONNET_FMT) "$$f" | diff -u "$$f" -; \
		done

	promtool check rules prometheus_alerts.yaml

clean:
	rm -rf dashboards_out prometheus_alerts.yaml
