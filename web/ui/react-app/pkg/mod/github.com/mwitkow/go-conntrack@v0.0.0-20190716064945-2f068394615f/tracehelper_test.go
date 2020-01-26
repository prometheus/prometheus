package conntrack_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/trace"
)

func fetchTraceEvents(t *testing.T, familyName string) string {
	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/debug/events?fam=%s&b=0&exp=1", familyName)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err, "failed creating request for Prometheus handler")
	trace.RenderEvents(resp, req, true)
	out, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err, "failed reading the trace page")
	return string(out)
}
