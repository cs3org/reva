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

package network

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	p "path"
)

type URLParams map[string]string

func GenerateURL(host string, path string, params URLParams) (*url.URL, error) {
	fullURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("unable to generate URL: base=%v, path=%v, params=%v", host, path, params)
	}

	fullURL.Path = p.Join(fullURL.Path, path)

	query := make(url.Values)
	for key, value := range params {
		query.Set(key, value)
	}
	fullURL.RawQuery = query.Encode()

	return fullURL, nil
}

func ReadEndpoint(host string, path string, params URLParams) ([]byte, error) {
	endpointURL, err := GenerateURL(host, path, params)
	if err != nil {
		return nil, fmt.Errorf("unable to generate endpoint URL: %v", err)
	}

	// Fetch the data and read the body
	resp, err := http.Get(endpointURL.String())
	if err != nil {
		return nil, fmt.Errorf("unable to get data from endpoint: %v", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("invalid response received: %v", resp.Status)
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	return body, nil
}
