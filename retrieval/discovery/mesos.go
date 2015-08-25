// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package discovery

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
	"github.com/prometheus/prometheus/config"
)

const (
	masterRedirectPath = "/master/redirect"
	masterStatePath    = "/master/state.json"
	metaLabelPrefix    = model.MetaLabelPrefix + "mesos_"

	hostnameLabel model.LabelName = metaLabelPrefix + "hostname"
	pidLabel      model.LabelName = metaLabelPrefix + "pid"
)

type mesosDiscovery struct {
	client            *http.Client
	clusterIdentifier string
	exporterPort      int
	masters           []*url.URL
	masterUrlPicker   func([]*url.URL) *url.URL
	refreshInterval   time.Duration
}

type slave struct {
	Active   bool
	Hostname string
	Pid      string
}

type state struct {
	Pid    string
	Slaves []*slave
}

// Run implements the TargetProvider interface.
func (m *mesosDiscovery) Run(ch chan<- *config.TargetGroup, done <-chan struct{}) {
	defer close(ch)

	ticker := time.NewTicker(m.refreshInterval)
	defer ticker.Stop()

	// Trigger the an update immediately.
	m.updateSlaves(ch)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			err := m.updateSlaves(ch)
			if err != nil {
				log.Errorf("Error while updating nodes: %s", err)
			}
		}
	}
}

// Sources implements the TargetProvider interface.
func (m *mesosDiscovery) Sources() []string {
	h := md5.New()
	for _, u := range m.masters {
		io.WriteString(h, u.Host)
	}
	m.clusterIdentifier = fmt.Sprintf("%x", h.Sum(nil))

	return []string{m.clusterIdentifier}
}

func (m *mesosDiscovery) fetchTargetGroup() (*config.TargetGroup, error) {
	s, err := m.fetchMasterState()
	if err != nil {
		return nil, err
	}

	targets := []model.LabelSet{}

	for _, sl := range s.Slaves {
		if !sl.Active {
			continue
		}
		pidParts := strings.Split(sl.Pid, "@")
		ip := strings.Split(pidParts[1], ":")[0]

		address := fmt.Sprintf("%s:%d", ip, m.exporterPort)

		ls := model.LabelSet{
			model.AddressLabel: model.LabelValue(address),
			hostnameLabel:      model.LabelValue(sl.Hostname),
			pidLabel:           model.LabelValue(sl.Pid),
		}

		targets = append(targets, ls)
	}

	tg := &config.TargetGroup{
		Targets: targets,
		Source:  m.clusterIdentifier,
	}

	return tg, nil
}

func (m *mesosDiscovery) updateSlaves(ch chan<- *config.TargetGroup) error {
	tg, err := m.fetchTargetGroup()
	if err != nil {
		return err
	}

	ch <- tg

	return nil
}

func (m *mesosDiscovery) fetchCurrentMaster() (*url.URL, error) {
	queryUrl := m.masterUrlPicker(m.masters)

	redirectUrl := fmt.Sprintf("%s://%s%s", queryUrl.Scheme, queryUrl.Host, masterRedirectPath)
	req, err := http.NewRequest("GET", redirectUrl, nil)
	if err != nil {
		return nil, err
	}

	tr := http.Transport{
		DisableKeepAlives: true,
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	masterLoc := resp.Header.Get("Location")
	if masterLoc == "" {
		return nil, errors.New("Redirect does not include the location of the Mesos Master leader")
	}

	// Starting from 0.23.0, the "Location" header does not include the protocol.
	if !strings.HasPrefix(masterLoc, "http") {
		masterLoc = fmt.Sprintf("%s:%s", queryUrl.Scheme, masterLoc)
	}

	masterUrl, err := url.Parse(masterLoc)
	if err != nil {
		return nil, err
	}
	return masterUrl, nil
}

func (m *mesosDiscovery) fetchMasterState() (*state, error) {
	masterUrl, err := m.fetchCurrentMaster()
	if err != nil {
		return nil, err
	}

	rep, err := m.client.Get(fmt.Sprintf("%s://%s%s", masterUrl.Scheme, masterUrl.Host, masterStatePath))
	if err != nil {
		return nil, err
	}
	defer rep.Body.Close()

	body, err := ioutil.ReadAll(rep.Body)
	if err != nil {
		return nil, err
	}

	s := &state{}
	err = json.Unmarshal(body, s)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func pickRandomMasterUrl(urls []*url.URL) *url.URL {
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)

	return urls[r.Intn(len(urls))]
}

// NewMesosDiscovery creates a new Mesos based discovery.
func NewMesosDiscovery(c *config.MesosSDConfig) *mesosDiscovery {
	var masters = []*url.URL{}

	for _, server := range c.Servers {
		u, _ := url.Parse(server)
		masters = append(masters, u)
	}

	return &mesosDiscovery{
		client:          &http.Client{},
		exporterPort:    c.ExporterPort,
		masters:         masters,
		masterUrlPicker: pickRandomMasterUrl,
		refreshInterval: time.Duration(c.RefreshInterval),
	}
}
