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

package preferencessvc

import (
	"context"
	"io"
	"sync"

	"google.golang.org/grpc"

	authproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/authprovider/v0alpha"
	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
)

type contextUserRequiredErr string

func (err contextUserRequiredErr) Error() string { return string(err) }

func init() {
	grpcserver.Register("preferencessvc", New)
}

// m maps user to map of user preferences.
// m = map[userToken]map[key]value
var m = make(map[string]map[string]string)

var mutex = &sync.Mutex{}

type service struct{}

// New returns a new PreferencesServiceServer
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {
	service := &service{}
	preferencesv0alphapb.RegisterPreferencesServiceServer(ss, service)
	return service, nil
}

func (s *service) Close() error {
	return nil
}

func getUser(ctx context.Context) (*authproviderv0alphapb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(contextUserRequiredErr("userrequired"), "preferencessvc: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (s *service) SetKey(ctx context.Context, req *preferencesv0alphapb.SetKeyRequest) (*preferencesv0alphapb.SetKeyResponse, error) {
	key := req.Key
	value := req.Val

	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "preferencessvc: failed to call getUser")
		return &preferencesv0alphapb.SetKeyResponse{
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

	return &preferencesv0alphapb.SetKeyResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetKey(ctx context.Context, req *preferencesv0alphapb.GetKeyRequest) (*preferencesv0alphapb.GetKeyResponse, error) {
	key := req.Key
	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "preferencessvc: failed to call getUser")
		return &preferencesv0alphapb.GetKeyResponse{
			Status: status.NewUnauthenticated(ctx, err, "user not found or invalid"),
		}, err
	}

	name := u.Username

	mutex.Lock()
	defer mutex.Unlock()
	if len(m[name]) != 0 {
		if value, ok := m[name][key]; ok {
			return &preferencesv0alphapb.GetKeyResponse{
				Status: status.NewOK(ctx),
				Val:    value,
			}, nil
		}
	}

	res := &preferencesv0alphapb.GetKeyResponse{
		Status: status.NewNotFound(ctx, "key not found"),
		Val:    "",
	}
	return res, nil
}
