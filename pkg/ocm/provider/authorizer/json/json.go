// Copyright 2018-2023 CERN
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
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/provider"
	"github.com/cs3org/reva/pkg/ocm/provider/authorizer/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

// New returns a new authorizer object.
func New(ctx context.Context, m map[string]interface{}) (provider.Authorizer, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	f, err := os.ReadFile(c.Providers)
	if err != nil {
		return nil, err
	}
	providers := []*ocmprovider.ProviderInfo{}
	err = json.Unmarshal(f, &providers)
	if err != nil {
		return nil, err
	}

	a := &authorizer{
		providerIPs: sync.Map{},
		conf:        &c,
	}
	a.providers = a.getOCMProviders(providers)

	return a, nil
}

type config struct {
	Providers             string `mapstructure:"providers"`
	VerifyRequestHostname bool   `mapstructure:"verify_request_hostname"`
}

func (c *config) ApplyTemplates() {
	if c.Providers == "" {
		c.Providers = "/etc/revad/ocm-providers.json"
	}
}

type authorizer struct {
	providers   []*ocmprovider.ProviderInfo
	providerIPs sync.Map
	conf        *config
}

func normalizeDomain(d string) (string, error) {
	var urlString string
	if strings.Contains(d, "://") {
		urlString = d
	} else {
		urlString = "https://" + d
	}

	u, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}

	return u.Hostname(), nil
}

func (a *authorizer) GetInfoByDomain(ctx context.Context, domain string) (*ocmprovider.ProviderInfo, error) {
	normalizedDomain, err := normalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	for _, p := range a.providers {
		if strings.Contains(p.Domain, normalizedDomain) {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(domain)
}

func (a *authorizer) IsProviderAllowed(ctx context.Context, pi *ocmprovider.ProviderInfo) error {
	var err error
	normalizedDomain, err := normalizeDomain(pi.Domain)
	if err != nil {
		return err
	}
	var providerAuthorized bool
	if normalizedDomain != "" {
		for _, p := range a.providers {
			if p.Domain == normalizedDomain {
				providerAuthorized = true
				break
			}
		}
	} else {
		providerAuthorized = true
	}

	switch {
	case !a.conf.VerifyRequestHostname:
		return nil
	case !providerAuthorized:
		return errtypes.NotFound(pi.GetDomain())
	case len(pi.Services) == 0:
		return errtypes.NotSupported("No IP provided")
	}

	var ocmHost string
	for _, p := range a.providers {
		if p.Domain == normalizedDomain {
			ocmHost, err = a.getOCMHost(p)
			if err != nil {
				return err
			}
			break
		}
	}
	if ocmHost == "" {
		return errtypes.InternalError("json: ocm host not specified for mesh provider")
	}

	providerAuthorized = false
	var ipList []string
	if hostIPs, ok := a.providerIPs.Load(ocmHost); ok {
		ipList = hostIPs.([]string)
	} else {
		addr, err := net.LookupIP(ocmHost)
		if err != nil {
			return errors.Wrap(err, "json: error looking up client IP")
		}
		for _, a := range addr {
			ipList = append(ipList, a.String())
		}
		a.providerIPs.Store(ocmHost, ipList)
	}

	for _, ip := range ipList {
		if ip == pi.Services[0].Host {
			providerAuthorized = true
			break
		}
	}
	if !providerAuthorized {
		return errtypes.NotFound("OCM Host")
	}

	return nil
}

func (a *authorizer) ListAllProviders(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	return a.providers, nil
}

func (a *authorizer) getOCMProviders(providers []*ocmprovider.ProviderInfo) (po []*ocmprovider.ProviderInfo) {
	for _, p := range providers {
		_, err := a.getOCMHost(p)
		if err == nil {
			po = append(po, p)
		}
	}
	return
}

func (a *authorizer) getOCMHost(pi *ocmprovider.ProviderInfo) (string, error) {
	for _, s := range pi.Services {
		if s.Endpoint.Type.Name == "OCM" {
			return s.Host, nil
		}
	}
	return "", errtypes.NotFound("OCM Host")
}
