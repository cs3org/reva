// Copyright 2018-2021 CERN
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
	"io/ioutil"
	"net"
	"net/url"
	"strings"
	"sync"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/provider"
	"github.com/cs3org/reva/pkg/ocm/provider/authorizer/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

// New returns a new authorizer object.
func New(m map[string]interface{}) (provider.Authorizer, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	c.init()

	f, err := ioutil.ReadFile(c.Providers)
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
		conf:        c,
	}
	a.providers = a.getOCMProviders(providers)

	return a, nil
}

type config struct {
	Providers             string `mapstructure:"providers"`
	VerifyRequestHostname bool   `mapstructure:"verify_request_hostname"`
}

func (c *config) init() {
	if c.Providers == "" {
		c.Providers = "/etc/revad/ocm-providers.json"
	}
}

type authorizer struct {
	providers   []*ocmprovider.ProviderInfo
	providerIPs sync.Map
	conf        *config
}

func (a *authorizer) GetInfoByDomain(ctx context.Context, domain string) (*ocmprovider.ProviderInfo, error) {
	for _, p := range a.providers {
		if strings.Contains(p.Domain, domain) {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(domain)
}

func (a *authorizer) IsProviderAllowed(ctx context.Context, provider *ocmprovider.ProviderInfo) error {

	var providerAuthorized bool
	if provider.Domain != "" {
		for _, p := range a.providers {
			if p.Domain == provider.Domain {
				providerAuthorized = true
				break
			}
		}
	} else {
		providerAuthorized = true
	}

	switch {
	case !providerAuthorized:
		return errtypes.NotFound(provider.GetDomain())
	case !a.conf.VerifyRequestHostname:
		return nil
	case len(provider.Services) == 0:
		return errtypes.NotSupported("No IP provided")
	}

	var ocmHost string
	var err error
	for _, p := range a.providers {
		if p.Domain == provider.Domain {
			ocmHost, err = a.getOCMHost(p)
			if err != nil {
				return err
			}
		}
	}
	if ocmHost == "" {
		return errors.Wrap(err, "json: ocm host not specified for mesh provider")
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
		if ip == provider.Services[0].Host {
			providerAuthorized = true
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

func (a *authorizer) getOCMHost(provider *ocmprovider.ProviderInfo) (string, error) {
	for _, s := range provider.Services {
		if s.Endpoint.Type.Name == "OCM" {
			ocmHost, err := url.Parse(s.Host)
			if err != nil {
				return "", errors.Wrap(err, "json: error parsing OCM host URL")
			}
			return ocmHost.Host, nil
		}
	}
	return "", errtypes.NotFound("OCM Host")
}
