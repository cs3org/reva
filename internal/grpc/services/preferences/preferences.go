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
	"sync"

	"google.golang.org/grpc"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
)

type contextUserRequiredErr string

func (err contextUserRequiredErr) Error() string { return string(err) }

func init() {
	rgrpc.Register("preferences", New)
}

// m maps user to map of user preferences.
// m = map[userToken]map[key]value
var m = make(map[string]map[string]string)

var mutex = &sync.Mutex{}

type service struct{}

// New returns a new PreferencesServiceServer
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	service := &service{}
	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	preferences.RegisterPreferencesAPIServer(ss, s)
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(contextUserRequiredErr("userrequired"), "preferences: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (s *service) SetKey(ctx context.Context, req *preferences.SetKeyRequest) (*preferences.SetKeyResponse, error) {
	key := req.Key
	value := req.Val

	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "preferences: failed to call getUser")
		return &preferences.SetKeyResponse{
			Status: status.NewUnauthenticated(ctx, err, "user not found or invalid"),
		}, err
	}

	name := u.Username

	mutex.Lock()
	defer mutex.Unlock()
	if len(m[name]) == 0 {
		m[name] = map[string]string{key: value}
	} else {
		usersettings := m[name]
		usersettings[key] = value
	}

	return &preferences.SetKeyResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetKey(ctx context.Context, req *preferences.GetKeyRequest) (*preferences.GetKeyResponse, error) {
	key := req.Key
	u, err := getUser(ctx)
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
		if value, ok := m[name][key]; ok {
			return &preferences.GetKeyResponse{
				Status: status.NewOK(ctx),
				Val:    value,
			}, nil
		}
	}

	res := &preferences.GetKeyResponse{
		Status: status.NewNotFound(ctx, "key not found"),
		Val:    "",
	}
	return res, nil
}
