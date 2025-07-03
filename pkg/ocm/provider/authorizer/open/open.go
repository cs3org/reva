// Copyright 2018-2024 CERN
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

package open

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	client "github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/ocm/provider"
	"github.com/cs3org/reva/v3/pkg/ocm/provider/authorizer/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("open", New)
}

// New returns a new authorizer object.
func New(ctx context.Context, m map[string]interface{}) (provider.Authorizer, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	a := &authorizer{}
	return a, nil
}

type config struct {
	// Users holds a path to a file containing json conforming the Users struct
	Providers string `mapstructure:"providers"`
}

func (c *config) ApplyDefaults() {
}

type authorizer struct {
	providers []*ocmprovider.ProviderInfo
}

func (a *authorizer) GetInfoByDomain(ctx context.Context, domain string) (*ocmprovider.ProviderInfo, error) {
	for _, p := range a.providers {
		if strings.Contains(p.Domain, domain) {
			return p, nil
		}
	}

	var endpoint string
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		endpoint = "https://" + domain
	} else {
		endpoint = domain
	}

	// not yet known: try to discover the remote OCM endpoint
	ocmClient := client.NewClient(time.Duration(10)*time.Second, true)
	ocmCaps, err := ocmClient.Discover(ctx, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "error probing OCM services at remote server")
	}
	var path string
	for _, t := range ocmCaps.ResourceTypes {
		webdavRoot, ok := t.Protocols["webdav"]
		if ok {
			// assume the first resourceType that exposes a webdav root is OK to use: as a matter of fact,
			// no implementation exists yet that exposes multiple resource types with different roots.
			path = filepath.Join(ocmCaps.Endpoint, webdavRoot)
		}
	}
	host, _ := url.Parse(ocmCaps.Endpoint)

	// return a provider info record for this domain, including the OCM service
	return &ocmprovider.ProviderInfo{
		Name:         "ocm_" + domain,
		FullName:     ocmCaps.Provider,
		Description:  "OCM service at " + domain,
		Organization: domain,
		Domain:       domain,
		Homepage:     "",
		Email:        "",
		Properties:   map[string]string{},
		Services: []*ocmprovider.Service{
			{
				Endpoint: &ocmprovider.ServiceEndpoint{
					Type: &ocmprovider.ServiceType{Name: "OCM"},
					Path: ocmCaps.Endpoint,
				},
				Host: host.Hostname(),
			},
			{
				Endpoint: &ocmprovider.ServiceEndpoint{
					Type: &ocmprovider.ServiceType{Name: "Webdav"},
					Path: path,
				},
				Host: host.Hostname(),
			},
		},
	}, nil
}

func (a *authorizer) IsProviderAllowed(ctx context.Context, provider *ocmprovider.ProviderInfo) error {
	return nil
}

func (a *authorizer) ListAllProviders(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	return a.providers, nil
}
