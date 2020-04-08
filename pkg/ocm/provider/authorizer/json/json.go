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

package json

import (
	"context"
	"encoding/json"
	"io/ioutil"

	ocmauthorizer "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
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

	f, err := ioutil.ReadFile(c.Providers)
	if err != nil {
		return nil, err
	}
	providers := []*ocmauthorizer.ProviderInfo{}
	err = json.Unmarshal(f, &providers)
	if err != nil {
		return nil, err
	}

	return &authorizer{
		providers: providers,
	}, nil
}

type config struct {
	// Users holds a path to a file containing json conforming the Users struct
	Providers string `mapstructure:"providers"`
}

type authorizer struct {
	providers []*ocmauthorizer.ProviderInfo
}

func (a *authorizer) IsProviderAllowed(ctx context.Context, domain string) error {
	for _, u := range a.providers {
		if u.Domain == domain {
			return nil
		}
	}
	return errtypes.NotFound(domain)
}
