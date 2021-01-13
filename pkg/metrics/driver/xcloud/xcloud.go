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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/cs3org/reva/pkg/metrics/driver/registry"

	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/metrics/config"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = logger.New().With().Int("pid", os.Getpid()).Logger()
	driver := &CloudDriver{CloudData: &CloudData{}}
	registry.Register(driverName(), driver)

	// TODO(labkode): spawn goroutines to fetch metrics and update the register service

	// get configuration from internal_metrics endpoint exposed
	// by the sciencemesh app

	client := &http.Client{}

	// endpoint example: https://mybox.com or https://mybox.com/owncloud
	endpoint := fmt.Sprintf("%s/index.php/apps/sciencemesh/internal_metrics", driver.CloudInstance)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Err(err).Msgf("xcloud: error creating request to %s", driver.CloudInstance)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Err(err).Msgf("xcloud: error getting internal metrics from %s", driver.CloudInstance)
		return
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("xcloud: error getting internal metrics from %s")
		log.Err(err).Msgf("xcloud: error getting internal metrics from %s", driver.CloudInstance)
		return
	}
	defer resp.Body.Close()

	// read response body
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Err(err).Msgf("xcloud: error reading resp body from internal metrics from %s", driver.CloudInstance)
	}

	cd := &CloudData{}
	if err := json.Unmarshal(data, cd); err != nil {
		log.Err(err).Msgf("xcloud: error parsing body from internal metrics: body(%s)", string(data))
	}

	fmt.Printf("%+v\n", cd)
}

func driverName() string {
	return "xcloud"
}

// CloudDriver the JsonDriver struct
type CloudDriver struct {
	CloudInstance   string
	RegisterService string
	CloudData       *CloudData
}

// Configure configures this driver
func (d *CloudDriver) Configure(c *config.Config) error {
	if c.CloudInstance == "" {
		err := errors.New("xcloud: missing cloud_instance config parameter")
		return err
	}

	d.CloudInstance = c.CloudInstance
	return nil
}

// GetNumUsers returns the number of site users
func (d *CloudDriver) GetNumUsers() int64 {
	return d.CloudData.Metrics.TotalUsers
}

// GetNumGroups returns the number of site groups
func (d *CloudDriver) GetNumGroups() int64 {
	return d.CloudData.Metrics.TotalGroups
}

// GetAmountStorage returns the amount of site storage used
func (d *CloudDriver) GetAmountStorage() int64 {
	return d.CloudData.Metrics.TotalStorage
}

type CloudData struct {
	Metrics  CloudDataMetrics  `json:"metrics"`
	Settings CloudDataSettings `json:"settings"`
}

type CloudDataMetrics struct {
	TotalUsers   int64 `json:"total_users"`
	TotalGroups  int64 `json:"total_groups"`
	TotalStorage int64 `json:"total_storage"`
}

type CloudDataSettings struct {
	IOPUrl   string `json:"iopurl"`
	Sitename string `json:"sitename"`
	Hostname string `json:"hostname"`
	Country  string `json:"country"`
}
