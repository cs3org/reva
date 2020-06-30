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
	"context"
	"encoding/json"
	"net/http"
	"strings"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/rhttp"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/provider"
	"github.com/cs3org/reva/pkg/ocm/provider/authorizer/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("mentix", New)
}

// Client is a Mentix API client
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a new authorizer object.
func New(m map[string]interface{}) (provider.Authorizer, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	c.init()

	client := &Client{
		BaseURL:    c.URL,
		HTTPClient: rhttp.GetHTTPClient(context.Background()),
	}

	return &authorizer{
		client: client,
		conf:   c,
	}, nil
}

type config struct {
	URL                   string `mapstructure:"url"`
	VerifyRequestHostname bool   `mapstructure:"verify_request_hostname"`
}

func (c *config) init() {
	if c.URL == "" {
		c.URL = "http://localhost:9600/mentix"
	}
}

type authorizer struct {
	client *Client
	conf   *config
}

func (c *Client) sendRequest(req *http.Request) (*provider.MentixResponse, error) {
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	// TODO(mirekys): use providerv1beta1.ListAllProvidersResponse directly after mentix service uses it
	var mr provider.MentixResponse
	if err = json.NewDecoder(res.Body).Decode(&mr); err != nil {
		return nil, err
	}

	return &mr, nil
}

func (c *Client) fetchAllProviders() ([]*ocmprovider.ProviderInfo, error) {
	req, err := http.NewRequest("GET", c.BaseURL, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	// TODO(mirekys): following translation won't be needed after mentix exporter also uses ocmprovider struct
	providers := make([]*ocmprovider.ProviderInfo, 0, len(res.Sites))
	for _, si := range res.Sites {
		services := make([]*ocmprovider.Service, 0, len(si.Services))
		for _, se := range si.Services {
			services = append(services, &ocmprovider.Service{
				Host: se.Host,
				Endpoint: &ocmprovider.ServiceEndpoint{
					Type: &ocmprovider.ServiceType{
						Name:        se.Type.Name,
						Description: se.Type.Description,
					},
					Name:        se.Name,
					Path:        se.URL,
					IsMonitored: se.IsMonitored,
				},
			})
		}

		providers = append(providers, &ocmprovider.ProviderInfo{
			Name:         si.Name,
			FullName:     si.FullName,
			Description:  si.Description,
			Organization: si.Organization,
			Domain:       si.Domain,
			Homepage:     si.Homepage,
			Email:        si.Email,
			Services:     services,
		})
	}

	status := &rpcv1beta1.Status{
		Code:    http.StatusOK,
		Message: "",
	}

	pi := &ocmprovider.ListAllProvidersResponse{
		Status:    status,
		Providers: providers,
	}

	if pi.Providers == nil {
		return nil, errtypes.InternalError(pi.Status.GetMessage())
	}
	return pi.Providers, nil
}

func (a *authorizer) GetInfoByDomain(ctx context.Context, domain string) (*ocmprovider.ProviderInfo, error) {
	providers, err := a.client.fetchAllProviders()
	if err != nil {
		return nil, err
	}

	for _, p := range providers {
		if strings.Contains(p.Domain, domain) {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(domain)
}

func (a *authorizer) IsProviderAllowed(ctx context.Context, provider *ocmprovider.ProviderInfo) error {
	providers, err := a.client.fetchAllProviders()
	if err != nil {
		return err
	}

	var providerAuthorized bool
	for _, p := range providers {
		if p.Domain == provider.GetDomain() {
			providerAuthorized = true
		}
	}

	if !providerAuthorized {
		return errtypes.NotFound(provider.GetDomain())
	} else if !a.conf.VerifyRequestHostname {
		return nil
	}

	providerAuthorized = false
	ocmHost, err := getOCMHost(provider)
	if err != nil {
		return errors.Wrap(err, "json: ocm host not specified for mesh provider")
	}

	for _, s := range provider.Services {
		if s.Host == ocmHost {
			providerAuthorized = true
		}
	}

	if !providerAuthorized {
		return errtypes.NotFound("OCM Host")
	}

	return nil
}

func (a *authorizer) ListAllProviders(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	res, err := a.client.fetchAllProviders()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func getOCMHost(originProvider *ocmprovider.ProviderInfo) (string, error) {
	for _, s := range originProvider.Services {
		if s.Endpoint.Type.Name == "OCM" {
			return s.Host, nil
		}
	}
	return "", errtypes.NotFound("OCM Host")
}
