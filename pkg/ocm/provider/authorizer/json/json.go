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

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/provider"
	"github.com/cs3org/reva/pkg/ocm/provider/authorizer/registry"
)

func init() {
	registry.Register("json", New)
}

// New returns a new authorizer object.
func New(m map[string]interface{}) (provider.Authorizer, error) {
	auth := new(authorizer)
	return auth, nil
}

type authorizer struct {
}

func (a *authorizer) IsProviderAllowed(ctx context.Context, domain string) error {
	return nil
}

func (a *authorizer) GetProviderInfoByDomain(ctx context.Context, domain string) (*ocm.ProviderInfo, error) {
	p := new(ocm.ProviderInfo)
	return p, nil
}

func (a *authorizer) AddProvider(ctx context.Context, p *ocm.ProviderInfo) error {
	return nil
}
