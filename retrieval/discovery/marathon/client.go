package marathon

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

// AppListClient defines a function that can be used to get an application list from marathon.
type AppListClient func(url string) (*AppList, error)

// FetchMarathonApps requests a list of applications from a marathon server.
func FetchMarathonApps(url string) (*AppList, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseAppJSON(body)
}

func parseAppJSON(body []byte) (*AppList, error) {
	apps := &AppList{}
	err := json.Unmarshal(body, apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}
