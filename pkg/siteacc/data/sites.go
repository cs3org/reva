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

package data

import (
	"encoding/json"
	"fmt"

	"github.com/cs3org/reva/pkg/mentix/utils/network"
	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/pkg/errors"
)

// QueryAvailableSites uses Mentix to query a list of all available (registered) sites.
func QueryAvailableSites(conf *config.Configuration) (map[string]string, error) {
	mentixURL, err := network.GenerateURL(conf.Mentix.URL, conf.Mentix.DataEndpoint, network.URLParams{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate Mentix URL")
	}

	data, err := network.ReadEndpoint(mentixURL, nil, true)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read the Mentix endpoint")
	}

	// Decode the data into a simplified, reduced data type
	type siteData struct {
		Sites []struct {
			ID   string
			Name string
		}
	}
	sites := siteData{}
	if err := json.Unmarshal(data, &sites); err != nil {
		fmt.Println(err)
		return nil, errors.Wrap(err, "error while decoding the JSON data")
	}

	siteEntries := make(map[string]string, len(sites.Sites))
	for _, site := range sites.Sites {
		siteEntries[site.ID] = site.Name
	}
	return siteEntries, nil
}

// QuerySiteName uses Mentix to query the name of a site given by its ID.
func QuerySiteName(siteID string, conf *config.Configuration) (string, error) {
	sites, err := QueryAvailableSites(conf)
	if err != nil {
		return "", err
	}

	if name, ok := sites[siteID]; ok {
		return name, nil
	}

	return "", errors.Errorf("no site with ID %v found", siteID)
}
