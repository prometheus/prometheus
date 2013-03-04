// Copyright 2013 Prometheus Team
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

package api

import (
	"code.google.com/p/gorest"
	"github.com/prometheus/prometheus/appstate"
	"github.com/prometheus/prometheus/utility"
)

type MetricsService struct {
	gorest.RestService `root:"/api/" consumes:"application/json" produces:"application/json"`

	query      gorest.EndPoint `method:"GET" path:"/query?{expr:string}&{json:string}" output:"string"`
	queryRange gorest.EndPoint `method:"GET" path:"/query_range?{expr:string}&{end:int64}&{range:int64}&{step:int64}" output:"string"`
	metrics    gorest.EndPoint `method:"GET" path:"/metrics" output:"string"`

	setTargets gorest.EndPoint `method:"PUT" path:"/jobs/{jobName:string}/targets" postdata:"[]TargetGroup"`

	appState *appstate.ApplicationState
	time     utility.Time
}

func NewMetricsService(appState *appstate.ApplicationState) *MetricsService {
	return &MetricsService{
		appState: appState,
	}
}
