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

package context

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/helper"
	"github.com/cs3org/reva/pkg/storage/pw/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("context", New)
}

type config struct {
	Prefix string `mapstructure:"prefix"`
	Layout string `mapstructure:"layout"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{Layout: "{{.Username}}"}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.PathWrapper interface that
// is used to wrap and unwrap storage paths
func New(m map[string]interface{}) (storage.PathWrapper, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &pw{prefix: c.Prefix, layout: c.Layout}, nil
}

type pw struct {
	prefix string
	layout string
}

// Only works when a user is in context
func (pw *pw) Unwrap(ctx context.Context, rp string) (string, error) {

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
	}
	if u.Username == "" {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), "user has no username")
	}
	userhome, err := helper.GetUserHomePath(u, pw.layout)
	if err != nil {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), fmt.Sprintf("template error: %s", err.Error()))
	}
	return path.Join("/", pw.prefix, userhome, rp), nil
}

func (pw *pw) Wrap(ctx context.Context, rp string) (string, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
	}
	if u.Username == "" {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), "user has no username")
	}
	userhome, err := helper.GetUserHomePath(u, pw.layout)
	if err != nil {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), fmt.Sprintf("template error: %s", err.Error()))
	}
	return strings.TrimPrefix(rp, path.Join("/", userhome)), nil
}
