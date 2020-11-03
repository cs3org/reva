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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	p "path"
	"strings"
)

// URLParams holds Key-Value URL parameters; it is a simpler form of url.Values.
type URLParams map[string]string

// ResponseParams holds parameters of an HTTP response.
type ResponseParams map[string]interface{}

// GenerateURL creates a URL object from a host, path and optional parameters.
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

// ReadEndpoint reads data from an HTTP endpoint.
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

// CreateResponse creates a generic HTTP response in JSON format.
func CreateResponse(msg string, params ResponseParams) []byte {
	if params == nil {
		params = make(map[string]interface{})
	}
	params["message"] = msg

	jsonData, _ := json.MarshalIndent(params, "", "\t")
	return jsonData
}

// ExtractDomainFromURL extracts the domain name (domain.tld) from a URL.
func ExtractDomainFromURL(hostURL *url.URL) string {
	// Remove host port if present
	host, _, err := net.SplitHostPort(hostURL.Host)
	if err != nil {
		host = hostURL.Host
	}

	// Remove subdomain
	if idx := strings.Index(host, "."); idx != -1 {
		host = host[idx+1:]
	}

	return host
}
