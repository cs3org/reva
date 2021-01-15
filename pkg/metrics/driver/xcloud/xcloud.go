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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

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

}

func driverName() string {
	return "xcloud"
}

// CloudDriver the JsonDriver struct
type CloudDriver struct {
	instance     string
	catalog      string
	pullInterval int
	CloudData    *CloudData
	sync.Mutex
}

func (d *CloudDriver) refresh() error {
	// TODO(labkode): spawn goroutines to fetch metrics and update the register service

	// get configuration from internal_metrics endpoint exposed
	// by the sciencemesh app
	client := &http.Client{}

	// endpoint example: https://mybox.com or https://mybox.com/owncloud
	endpoint := fmt.Sprintf("%s/index.php/apps/sciencemesh/internal_metrics", d.instance)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Err(err).Msgf("xcloud: error creating request to %s", d.instance)
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Err(err).Msgf("xcloud: error getting internal metrics from %s", d.instance)
		return err
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("xcloud: error getting internal metrics from %s. http status code (%d)", resp.StatusCode)
		log.Err(err).Msgf("xcloud: error getting internal metrics from %s", d.instance)
		return err
	}
	defer resp.Body.Close()

	// read response body
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Err(err).Msgf("xcloud: error reading resp body from internal metrics from %s", d.instance)
		return err
	}

	cd := &CloudData{}
	if err := json.Unmarshal(data, cd); err != nil {
		log.Err(err).Msgf("xcloud: error parsing body from internal metrics: body(%s)", string(data))
		return err
	}

	d.Lock()
	defer d.Unlock()
	d.CloudData = cd
	log.Info().Msgf("xcloud: received internal metrics from cloud provider: %+v", cd)

	mc := &MentixCatalog{
		Name:        cd.Settings.Sitename,
		FullName:    cd.Settings.Sitename,
		Homepage:    cd.Settings.Hostname,
		Description: "ScienceMesh App from " + cd.Settings.Sitename,
		CountryCode: cd.Settings.Country,
		Services: []*MentixService{
			&MentixService{
				Host:        cd.Settings.Hostname,
				IsMonitored: true,
				Name:        cd.Settings.Hostname + " - REVAD",
				URL:         cd.Settings.Siteurl,
				Properties: &MentixServiceProperties{
					MetricsPath: fmt.Sprintf("%s/index.php/apps/sciencemesh/metrics", cd.Settings.Siteurl),
				},
				Type: &MentixServiceType{
					Name: "REVAD",
				},
			},
		},
	}

	j, err := json.Marshal(mc)
	if err != nil {
		log.Err(err).Msgf("xcloud: error marhsaling mentix calalog info")
		return err
	}

	log.Info().Msgf("xcloud: info to send to register: %s", string(j))

	// send to register if catalog is set
	req, err = http.NewRequest("POST", d.catalog, bytes.NewBuffer(j))
	resp, err = client.Do(req)
	if err != nil {
		log.Err(err).Msgf("xcloud: error registering catalog info to: %s with info: %s", d.catalog, string(j))
		return err
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	log.Info().Msgf("xcloud: site registered: %s", string(body))

	return nil
}

// Configure configures this driver
func (d *CloudDriver) Configure(c *config.Config) error {
	if c.XcloudInstance == "" {
		err := errors.New("xcloud: missing xcloud_instance config parameter")
		return err
	}

	if c.XcloudPullInterval == 0 {
		c.XcloudPullInterval = 10 // seconds
	}

	d.instance = c.XcloudInstance
	d.pullInterval = c.XcloudPullInterval
	d.catalog = c.XcloudCatalog

	ticker := time.NewTicker(time.Duration(d.pullInterval) * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				d.refresh()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

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
	Siteurl  string `json:"siteurl"`
	Hostname string `json:"hostname"`
	Country  string `json:"country"`
}

type MentixCatalog struct {
	CountryCode string           `json:"CountryCode"`
	Description string           `json:"Description"`
	FullName    string           `json:"FullName"`
	Homepage    string           `json:"Homepage"`
	Name        string           `json:"Name"`
	Services    []*MentixService `json:"Services"`
}

type MentixService struct {
	Host        string                   `json:"Host"`
	IsMonitored bool                     `json:"IsMonitored"`
	Name        string                   `json:"Name"`
	Properties  *MentixServiceProperties `json:"Properties"`
	Type        *MentixServiceType       `json:"Type"`
	URL         string                   `json:"URL"`
}

type MentixServiceProperties struct {
	MetricsPath string `json:"METRICS_PATH"`
}
type MentixServiceType struct {
	Name string `json:"Name"`
}
