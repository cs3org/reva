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

package main

import (
	"context"
	"io"
	"reflect"
	"strings"
	"time"

	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	share "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth/scope"
	"github.com/cs3org/reva/pkg/errtypes"
)

type AppTokenCreateOpts struct {
	Expiration string
	Label      string
	Path       string
	Share      string
	Unlimited  bool
}

var appTokensCreateOpts *AppTokenCreateOpts = &AppTokenCreateOpts{}

const layoutTime = "2006-01-02T15:04"

func appTokensCreateCommand() *command {
	cmd := newCommand("token-create")
	cmd.Description = func() string { return "create a new application tokens" }
	cmd.Usage = func() string { return "Usage: token-create" }

	cmd.StringVar(&appTokensCreateOpts.Label, "label", "", "set a label")
	cmd.StringVar(&appTokensCreateOpts.Expiration, "expiration", "", "set expiration time (format <yyyy-mm-dd hh:mm>)")
	// TODO(gmgigi96): add support for multiple paths and shares for the same token
	cmd.StringVar(&appTokensCreateOpts.Path, "path", "", "create a token on a file (format path:[r|w])")
	cmd.StringVar(&appTokensCreateOpts.Share, "share", "", "create a token for a share (format shareid:[r|w])")
	cmd.BoolVar(&appTokensCreateOpts.Unlimited, "all", false, "create a token with an unlimited scope")

	cmd.ResetFlags = func() {
		s := reflect.ValueOf(appTokensCreateOpts).Elem()
		s.Set(reflect.Zero(s.Type()))
	}

	cmd.Action = func(w ...io.Writer) error {

		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()

		scope, err := getScope(ctx, client, appTokensCreateOpts)
		if err != nil {
			return err
		}

		// parse eventually expiration time
		var expiration *types.Timestamp
		if appTokensCreateOpts.Expiration != "" {
			exp, err := time.Parse(layoutTime, appTokensCreateOpts.Expiration)
			if err != nil {
				return err
			}
			expiration = &types.Timestamp{
				Seconds: uint64(exp.Unix()),
			}
		}

		client.GenerateAppPassword(ctx, &authapp.GenerateAppPasswordRequest{
			Expiration: expiration,
			Label:      appTokensCreateOpts.Label,
			TokenScope: scope,
		})

		return nil
	}

	return cmd
}

func getScope(ctx context.Context, client gateway.GatewayAPIClient, opts *AppTokenCreateOpts) (map[string]*authpb.Scope, error) {
	switch {
	case opts.Share != "":
		// TODO(gmgigi96): verify format
		// share = xxxx:[r|w]
		shareIDPerm := strings.Split(opts.Share, ":")
		shareID, perm := shareIDPerm[0], shareIDPerm[1]
		return getPublicShareScope(ctx, client, shareID, perm)
	case opts.Path != "":
		// TODO(gmgigi96): verify format
		// path = /home/a/b:[r|w]
		pathPerm := strings.Split(opts.Path, ":")
		path, perm := pathPerm[0], pathPerm[1]
		return getPathScope(ctx, client, path, perm)
	case opts.Unlimited:
		return scope.GetOwnerScope()
	}

	return nil, nil
}

func getPublicShareScope(ctx context.Context, client gateway.GatewayAPIClient, shareID, perm string) (map[string]*authpb.Scope, error) {
	role, err := parsePermission(perm)
	if err != nil {
		return nil, err
	}

	publicShareResponse, err := client.GetPublicShare(ctx, &share.GetPublicShareRequest{
		Ref: &share.PublicShareReference{
			Spec: &share.PublicShareReference_Id{
				Id: &share.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	})

	if err != nil {
		return nil, err
	}
	if publicShareResponse.Status.Code != rpc.Code_CODE_OK {
		return nil, formatError(publicShareResponse.Status)
	}

	return scope.GetPublicShareScope(publicShareResponse.GetShare(), role)
}

func getPathScope(ctx context.Context, client gateway.GatewayAPIClient, path, perm string) (map[string]*authpb.Scope, error) {
	role, err := parsePermission(perm)
	if err != nil {
		return nil, err
	}

	statResponse, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path,
			},
		},
	})

	if err != nil {
		return nil, err
	}
	if statResponse.Status.Code != rpc.Code_CODE_OK {
		return nil, formatError(statResponse.Status)
	}

	return scope.GetResourceInfoScope(statResponse.GetInfo(), role)
}

// parse permission string in the form of "rw" to create a role
func parsePermission(perm string) (authpb.Role, error) {
	switch perm {
	case "r":
		return authpb.Role_ROLE_VIEWER, nil
	case "w":
		return authpb.Role_ROLE_EDITOR, nil
	default:
		return authpb.Role_ROLE_INVALID, errtypes.BadRequest("not recognised permission")
	}
}
