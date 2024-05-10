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
	"strings"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/provider"
	"github.com/cs3org/reva/pkg/ocm/provider/authorizer/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
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
	// not yet known: try to discover the remote OCM endpoint
	//TODO
	// return a fake provider info record for this domain, including the OCM service
	return &ocmprovider.ProviderInfo{
		Name:         "ocm_" + domain,
		FullName:     "",
		Description:  "OCM service at " + domain,
		Organization: domain,
		Domain:       domain,
		Homepage:     "https://" + domain,
		Email:        "",
		Properties:   map[string]string{},
		Services: []*ocmprovider.Service{{
			Endpoint: &ocmprovider.ServiceEndpoint{
				Type: &ocmprovider.ServiceType{Name: "OCM"},
				Path: "",
			},
			Host: "",
		}},
	}, nil
}

func (a *authorizer) IsProviderAllowed(ctx context.Context, provider *ocmprovider.ProviderInfo) error {
	return nil
}

func (a *authorizer) ListAllProviders(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	return a.providers, nil
}
