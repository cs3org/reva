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
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/mentix/utils/network"
)

type accountsServiceSettings struct {
	URL      *url.URL
	user     string
	password string
}

type requestResponse struct {
	Success bool
	Error   string
	Data    interface{}
}

var (
	accountsService accountsServiceSettings
)

// ConfigureAccountsService configures the accounts service for the site registration importer.
func ConfigureAccountsService(address string, user string, password string) error {
	endpointURL, err := url.Parse(address)
	if err != nil {
		return errors.Wrap(err, "unable to parse the accounts service URL")
	}

	accountsService.URL = endpointURL
	accountsService.user = user
	accountsService.password = password

	return nil
}

func queryAccountsService(endpoint string, params network.URLParams) (*requestResponse, error) {
	fullURL, err := network.GenerateURL(fmt.Sprintf("%v://%v", accountsService.URL.Scheme, accountsService.URL.Host), path.Join(accountsService.URL.Path, endpoint), params)
	if err != nil {
		return nil, errors.Wrap(err, "error while building the service accounts query URL")
	}

	data, err := network.ReadEndpoint(fullURL, &network.BasicAuth{User: accountsService.user, Password: accountsService.password}, false)
	if err != nil {
		return nil, errors.Wrap(err, "unable to query the service accounts endpoint")
	}

	resp := &requestResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal response data")
	}
	return resp, nil
}

func getResponseValue(resp *requestResponse, path string) interface{} {
	if data, ok := resp.Data.(map[string]interface{}); ok {
		tokens := strings.Split(path, ".")
		for i, name := range tokens {
			if i == len(tokens)-1 {
				if value, ok := data[name]; ok {
					return value
				}
			}

			if data, ok = data[name].(map[string]interface{}); !ok {
				break
			}

		}
	}

	return nil
}
