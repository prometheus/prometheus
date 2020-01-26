package conntrack_test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
)

func fetchPrometheusLines(t *testing.T, metricName string, matchingLabelValues ...string) []string {
	resp := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/", nil)
	require.NoError(t, err, "failed creating request for Prometheus handler")
	promhttp.Handler().ServeHTTP(resp, req)
	reader := bufio.NewReader(resp.Body)
	ret := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		} else {
			require.NoError(t, err, "error reading stuff")
		}
		if !strings.HasPrefix(line, metricName) {
			continue
		}
		matches := true
		for _, labelValue := range matchingLabelValues {
			if !strings.Contains(line, `"`+labelValue+`"`) {
				matches = false
			}
		}
		if matches {
			ret = append(ret, line)
		}

	}
	return ret
}

func sumCountersForMetricAndLabels(t *testing.T, metricName string, matchingLabelValues ...string) int {
	count := 0
	for _, line := range fetchPrometheusLines(t, metricName, matchingLabelValues...) {
		valueString := line[strings.LastIndex(line, " ")+1 : len(line)-1]
		valueFloat, err := strconv.ParseFloat(valueString, 32)
		require.NoError(t, err, "failed parsing value for line: %v", line)
		count += int(valueFloat)
	}
	return count
}
