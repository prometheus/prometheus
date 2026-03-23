package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	e2einteractive "github.com/efficientgo/e2e/interactive"
	"github.com/stretchr/testify/require"
)

func TestMain_PromQLCompatDemo(t *testing.T) {
	dir, err := os.Getwd()
	require.NoError(t, err)

	open := func(port string) {
		time.Sleep(10 * time.Second)
		e2einteractive.OpenInBrowser(`http://localhost:` + port + `/query?g0.expr=sum%28prometheus_http_request_duration_seconds_bucket%29+by+%28job%2C+le%29&g0.show_tree=0&g0.tab=table&g0.range_input=1h&g0.res_type=auto&g0.res_density=medium&g0.display_mode=lines&g0.show_exemplars=0&g1.expr=sum%28prometheus_http_request_duration_seconds%29+by+%28job%29&g1.show_tree=0&g1.tab=table&g1.range_input=1h&g1.res_type=auto&g1.res_density=medium&g1.display_mode=lines&g1.show_exemplars=0`)
		e2einteractive.OpenInBrowser(`http://localhost:` + port + `/query?g0.expr=histogram_quantile%280.99%2C+sum%28prometheus_http_request_duration_seconds_bucket%29+by+%28job%2C+le%29%29&g0.show_tree=0&g0.tab=graph&g0.range_input=1h&g0.res_type=auto&g0.res_density=medium&g0.display_mode=lines&g0.show_exemplars=0&g1.expr=histogram_quantile%280.99%2C+sum%28prometheus_http_request_duration_seconds%29+by+%28job%29%29&g1.show_tree=0&g1.tab=graph&g1.range_input=1h&g1.res_type=auto&g1.res_density=medium&g1.display_mode=lines&g1.show_exemplars=0`)
		e2einteractive.OpenInBrowser(`http://localhost:` + port + `/query?g0.expr=sum%28prometheus_http_request_duration_seconds_count%29+by+%28job%29&g0.show_tree=0&g0.tab=graph&g0.range_input=1h&g0.res_type=auto&g0.res_density=medium&g0.display_mode=lines&g0.show_exemplars=0&g1.expr=histogram_count%28sum%28prometheus_http_request_duration_seconds%29+by+%28job%29%29&g1.show_tree=0&g1.tab=graph&g1.range_input=1h&g1.res_type=auto&g1.res_density=medium&g1.display_mode=lines&g1.show_exemplars=0`)
	}
	require.NoError(t, os.Chdir("../../")) // Ensure UI is sourced.

	t.Run("normal", func(t *testing.T) {
		go open("1234")

		cfgFile := filepath.Join(dir, "kubecon.yaml")
		data := filepath.Join(dir, "data", "normal")
		os.Args = []string{"main", "--web.listen-address=0.0.0.0:1234", "--config.file=" + cfgFile, "--storage.tsdb.path=" + data}
		main()
	})
	t.Run("compatible", func(t *testing.T) {
		go open("1235")

		cfgFile := filepath.Join(dir, "kubecon_compat.yaml")
		data := filepath.Join(dir, "data", "compatible")
		os.Args = []string{"main", "--web.listen-address=0.0.0.0:1235", "--config.file=" + cfgFile, "--storage.tsdb.path=" + data, "--enable-feature=promql-nhcb-as-classic"}
		main()
	})
}
