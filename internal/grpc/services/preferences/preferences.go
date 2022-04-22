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

package preferences

import (
	"context"

	"google.golang.org/grpc"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/pkg/errors"
)

func init() {
	rgrpc.Register("preferences", New)
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "memory"
	}
}

type service struct {
	conf *config
	pm   preferences.Manager
}

func getPreferencesManager(c *config) (preferences.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new PreferencesServiceServer
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	c.init()

	pm, err := getPreferencesManager(c)
	if err != nil {
		return nil, err
	}

	return &service{
		conf: c,
		pm:   pm,
	}, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	preferencespb.RegisterPreferencesAPIServer(ss, s)
}

func (s *service) SetKey(ctx context.Context, req *preferencespb.SetKeyRequest) (*preferencespb.SetKeyResponse, error) {
	err := s.pm.SetKey(ctx, req.Key.Key, req.Key.Namespace, req.Val)
	if err != nil {
		return &preferencespb.SetKeyResponse{
			Status: status.NewInternal(ctx, err, "error setting key"),
		}, nil
	}

	name := u.Username

	mutex.Lock()
	defer mutex.Unlock()
	if len(m[name]) == 0 {
		m[name] = map[string]string{key.Key: value}
	} else {
		usersettings := m[name]
		usersettings[key.Key] = value
	}

	return &preferences.SetKeyResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetKey(ctx context.Context, req *preferencespb.GetKeyRequest) (*preferencespb.GetKeyResponse, error) {
	val, err := s.pm.GetKey(ctx, req.Key.Key, req.Key.Namespace)
	if err != nil {
		err = errors.Wrap(err, "preferences: failed to call getUser")
		return &preferences.GetKeyResponse{
			Status: status.NewUnauthenticated(ctx, err, "user not found or invalid"),
		}, err
	}

	name := u.Username

	mutex.Lock()
	defer mutex.Unlock()
	if len(m[name]) != 0 {
		if value, ok := m[name][key.Key]; ok {
			return &preferences.GetKeyResponse{
				Status: status.NewOK(ctx),
				Val:    value,
			}, nil
		}
		return &preferencespb.GetKeyResponse{
			Status: st,
		}, nil
	}

	return &preferencespb.GetKeyResponse{
		Status: status.NewOK(ctx),
		Val:    val,
	}, nil
}
