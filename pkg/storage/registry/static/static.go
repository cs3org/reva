// Copyright 2018-2019 CERN
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

package static

import (
	"context"
	"strings"

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("static", New)
}

type reg struct {
	rules map[string]string
}

func (b *reg) ListProviders(ctx context.Context) ([]*storagetypespb.ProviderInfo, error) {
	providers := []*storagetypespb.ProviderInfo{}
	for k, v := range b.rules {
		providers = append(providers, &storagetypespb.ProviderInfo{
			Address:      v,
			ProviderPath: k,
		})
	}
	return providers, nil
}

// returns the the root path of the first provider in the list.
// TODO(labkode): this is not production ready.
func (b *reg) GetHome(ctx context.Context) (string, error) {
	for k := range b.rules {
		if strings.HasPrefix(k, "/") {
			return k, nil
		}
	}
	return "", errors.New("static: home not found")
}

func (b *reg) FindProvider(ctx context.Context, ref *storageproviderv0alphapb.Reference) (*storagetypespb.ProviderInfo, error) {
	// find longest match
	var match string

	// we try to find first by path as most storage operations will be done on path.
	fn := ref.GetPath()
	if fn != "" {
		for prefix := range b.rules {
			if strings.HasPrefix(fn, prefix) && len(prefix) > len(match) {
				match = prefix
			}
		}
	}

	if match != "" {
		return &storagetypespb.ProviderInfo{
			ProviderPath: match,
			Address:      b.rules[match],
		}, nil
	}

	// we try with id
	id := ref.GetId()
	if id == nil {
		return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
	}

	for prefix := range b.rules {
		if id.StorageId == prefix {
			// TODO(labkode): fill path info based on provider id, if path and storage id points to same id, take that.
			return &storagetypespb.ProviderInfo{
				ProviderId: prefix,
				Address:    b.rules[prefix],
			}, nil
		}
	}
	return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
}

type config struct {
	Rules map[string]string `mapstructure:"rules"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &reg{rules: c.Rules}, nil
}
