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

package sitereg

import (
	"net/url"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/mentix/key"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
	"github.com/cs3org/reva/pkg/mentix/utils/countries"
	"github.com/cs3org/reva/pkg/mentix/utils/network"
)

type siteRegistrationData struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	CountryCode string `json:"countryCode"`

	Reva struct {
		Host        string `json:"host"`
		URL         string `json:"url"`
		MetricsPath string `json:"metricsPath"`
	} `json:"reva"`
}

/* Example JSON:
{
  "name": "Testsite",
  "url": "https://test-site.de/owncloud",
  "countryCode": "DE",
  "reva": {
    "url": "https://test-site.de/owncloud/reva",
    "metricsPath": "/iop/metrics"
  }
}
*/

// Verify checks whether the entered data is valid and complete.
func (siteData *siteRegistrationData) Verify() error {
	if len(siteData.Name) == 0 {
		return errors.Errorf("no site name provided")
	}
	if len(siteData.URL) > 0 {
		if _, err := url.Parse(siteData.URL); err != nil {
			return errors.Wrap(err, "invalid site URL provided")
		}
	} else {
		return errors.Errorf("no site URL provided")
	}

	if len(siteData.Reva.Host) == 0 && len(siteData.Reva.URL) == 0 {
		return errors.Errorf("no Reva host or URL provided")
	}
	if len(siteData.Reva.URL) > 0 {
		if _, err := url.Parse(siteData.Reva.URL); err != nil {
			return errors.Wrap(err, "invalid Reva URL provided")
		}
	}
	if len(siteData.Reva.MetricsPath) == 0 {
		return errors.Errorf("no Reva metrics path provided")
	}

	return nil
}

// ToMeshDataSite converts the stored data into a meshdata site object, filling out as much data as possible.
func (siteData *siteRegistrationData) ToMeshDataSite(siteID key.SiteIdentifier, siteType meshdata.SiteType, email string) (*meshdata.Site, error) {
	siteURL, err := url.Parse(siteData.URL)
	if err != nil {
		return nil, errors.Wrap(err, "invalid site URL")
	}

	// Create the Reva service entry
	revaHost := siteData.Reva.Host
	revaURL := siteData.Reva.URL

	if len(revaHost) == 0 { // Infer host from URL
		URL, _ := url.Parse(revaURL)
		revaHost = network.ExtractDomainFromURL(URL, true)
	} else if len(revaURL) == 0 { // Infer URL from host
		URL, _ := network.GenerateURL(revaHost, "", network.URLParams{})
		revaURL = URL.String()
	}

	properties := make(map[string]string, 1)
	meshdata.SetPropertyValue(&properties, meshdata.PropertyMetricsPath, siteData.Reva.MetricsPath)

	revaService := &meshdata.Service{
		ServiceEndpoint: &meshdata.ServiceEndpoint{
			Type: &meshdata.ServiceType{
				Name:        "REVAD",
				Description: "Reva Daemon",
			},
			Name:        revaHost + " - REVAD",
			URL:         revaURL,
			IsMonitored: true,
			Properties:  properties,
		},
		Host:                revaHost,
		AdditionalEndpoints: nil,
	}

	// Create the site data
	site := &meshdata.Site{
		Type:         siteType,
		ID:           siteID,
		Name:         siteData.Name,
		FullName:     siteData.Name,
		Organization: "",
		Domain:       network.ExtractDomainFromURL(siteURL, true),
		Homepage:     siteData.URL,
		Email:        email,
		Description:  siteData.Name + " @ " + siteData.URL,
		Country:      countries.LookupCountry(siteData.CountryCode),
		CountryCode:  siteData.CountryCode,
		Location:     "",
		Latitude:     0,
		Longitude:    0,
		Services:     []*meshdata.Service{revaService},
		Properties:   nil,
	}

	site.InferMissingData()
	return site, nil
}
