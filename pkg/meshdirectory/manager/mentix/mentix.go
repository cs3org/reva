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

package mentix

import (
	"encoding/json"
	"fmt"
	"github.com/cs3org/reva/pkg/meshdirectory"
	"github.com/cs3org/reva/pkg/meshdirectory/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"net/http"
)

func init() {
	registry.Register("mentix", New)
}

// Client is a Mentix API client
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a new mesh directory manager object.
func New(m map[string]interface{}) (meshdirectory.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	client := &Client{
		BaseURL:    c.URL,
		HTTPClient: rhttp.GetHTTPClient(nil),
	}

	mgr := &mgr{
		cfg: c,
		client: client,
	}

	return mgr, nil
}

type config struct {
	URL string `mapstructure:"url"`
}

type mgr struct {
	client *Client
	cfg    *config
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) sendRequest(req *http.Request) (*meshdirectory.MentixResponse, error) {
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		var errRes meshdirectory.MentixErrorResponse
		if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
			return nil, errors.New(errRes.Message)
		}

		return nil, fmt.Errorf("unknown api error, status code: %d", res.StatusCode)
	}

	var mr meshdirectory.MentixResponse
	if err = json.NewDecoder(res.Body).Decode(&mr); err != nil {
		return nil, err
	}

	return &mr, nil
}

// GetMeshProviders gets the available mesh providers data.
func (m *mgr) GetMeshProviders() (*[]meshdirectory.MeshProvider, error) {
	req, err := http.NewRequest("GET", m.client.BaseURL, nil)
	if err != nil {
		return nil, err
	}

	res, err := m.client.sendRequest(req)
	if err != nil {
		return nil, err
	}

	// Filter out providers without OCM API service
	var providers []meshdirectory.MeshProvider
	for _, p := range res.Sites {
		for _, s := range p.Services {
			if s.Type.Name == "OCM" {
				providers = append(providers, p)
			}
		}
	}

	return &providers, nil
}
