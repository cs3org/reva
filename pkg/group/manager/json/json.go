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

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/group"
	"github.com/cs3org/reva/pkg/group/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

type manager struct {
	groups []*grouppb.Group
}

type config struct {
	// Groups holds a path to a file containing json conforming to the Groups struct
	Groups string `mapstructure:"groups"`
}

func (c *config) init() {
	if c.Groups == "" {
		c.Groups = "/etc/revad/groups.json"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	c.init()
	return c, nil
}

// New returns a group manager implementation that reads a json file to provide group metadata.
func New(m map[string]interface{}) (group.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	f, err := ioutil.ReadFile(c.Groups)
	if err != nil {
		return nil, err
	}

	groups := []*grouppb.Group{}

	err = json.Unmarshal(f, &groups)
	if err != nil {
		return nil, err
	}

	return &manager{
		groups: groups,
	}, nil
}

func (m *manager) GetGroup(ctx context.Context, gid *grouppb.GroupId) (*grouppb.Group, error) {
	return nil, errtypes.NotSupported("json: GetGroup is not implemented")
}

func (m *manager) GetGroupByClaim(ctx context.Context, claim, value string) (*grouppb.Group, error) {
	return nil, errtypes.NotSupported("json: GetGroupByClaim is not implemented")
}

func (m *manager) FindGroups(ctx context.Context, query string) ([]*grouppb.Group, error) {
	return nil, errtypes.NotSupported("json: FindGroups is not implemented")
}

func (m *manager) GetMembers(ctx context.Context, gid *grouppb.GroupId) ([]*userpb.UserId, error) {
	return nil, errtypes.NotSupported("json: GetMembers is not implemented")
}

func (m *manager) HasMember(ctx context.Context, gid *grouppb.GroupId, uid *userpb.UserId) (bool, error) {
	return false, errtypes.NotSupported("json: HasMember is not implemented")
}
