// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package json

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"reva/pkg/metrics/config"
)

// New returns a new MetricsJSONDriver object.
// It reads the data file from the specified config.MetricsDataLocation upon initializing.
// It does not reload the data file for each metric.
func New(config *config.Config) (*MetricsJSONDriver, error) {
	// the json driver reads the data metrics file upon initializing
	metricsData, err := readJSON(config)
	if err != nil {
		return nil, err
	}

	driver := &MetricsJSONDriver{
		config: config,
		data:   metricsData,
	}

	return driver, nil
}

func readJSON(config *config.Config) (*data, error) {
	if config.MetricsDataLocation == "" {
		err := errors.New("Unable to initialize a metrics data driver, has the data location (metrics_data_location) been configured?")
		return nil, err
	}

	file, err := ioutil.ReadFile(config.MetricsDataLocation)
	if err != nil {
		return nil, err
	}

	data := &data{}
	err = json.Unmarshal(file, data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type data struct {
	NumUsers      int64 `json:"cs3_org_sciencemesh_site_total_num_users"`
	NumGroups     int64 `json:"cs3_org_sciencemesh_site_total_num_groups"`
	AmountStorage int64 `json:"cs3_org_sciencemesh_site_total_amount_storage"`
}

// MetricsJSONDriver the JsonDriver struct that also holds the data
type MetricsJSONDriver struct {
	config *config.Config
	data   *data
}

// GetNumUsers returns the number of site users
func (d *MetricsJSONDriver) GetNumUsers() int64 {
	return int64(d.data.NumUsers)
}

// GetNumGroups returns the number of site groups
func (d *MetricsJSONDriver) GetNumGroups() int64 {
	return int64(d.data.NumGroups)
}

// GetAmountStorage returns the amount of site storage used
func (d *MetricsJSONDriver) GetAmountStorage() int64 {
	return int64(d.data.AmountStorage)
}
