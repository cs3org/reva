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

type appTokenCreateOpts struct {
	Expiration string
	Label      string
	Path       stringSlice
	Share      stringSlice
	Unlimited  bool
}

type stringSlice []string

func (ss *stringSlice) Set(value string) error {
	*ss = append(*ss, value)
	return nil
}

func (ss *stringSlice) String() string {
	return strings.Join([]string(*ss), ",")
}

const layoutTime = "2006-01-02"

func appTokensCreateCommand() *command {
	cmd := newCommand("app-tokens-create")
	cmd.Description = func() string { return "create a new application tokens" }
	cmd.Usage = func() string { return "Usage: token-create" }

	var path, share stringSlice
	label := cmd.String("label", "", "set a label")
	expiration := cmd.String("expiration", "", "set expiration time (format <yyyy-mm-dd>)")
	cmd.Var(&path, "path", "create a token for a file (format path:[r|w]). It is possible specify this flag multiple times")
	cmd.Var(&share, "share", "create a token for a share (format shareid:[r|w]). It is possible specify this flag multiple times")
	unlimited := cmd.Bool("all", false, "create a token with an unlimited scope")

	cmd.ResetFlags = func() {
		path, share, label, expiration, unlimited = nil, nil, nil, nil, nil
	}

	cmd.Action = func(w ...io.Writer) error {
		createOpts := &appTokenCreateOpts{
			Expiration: *expiration,
			Label:      *label,
			Path:       path,
			Share:      share,
			Unlimited:  *unlimited,
		}

		err := checkOpts(createOpts)
		if err != nil {
			return err
		}

		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()

		scope, err := getScope(ctx, client, createOpts)
		if err != nil {
			return err
		}

		// parse eventually expiration time
		var expiration *types.Timestamp
		if createOpts.Expiration != "" {
			exp, err := time.Parse(layoutTime, createOpts.Expiration)
			if err != nil {
				return err
			}
			expiration = &types.Timestamp{
				Seconds: uint64(exp.Unix()),
			}
		}

		generateAppPasswordResponse, err := client.GenerateAppPassword(ctx, &authapp.GenerateAppPasswordRequest{
			Expiration: expiration,
			Label:      createOpts.Label,
			TokenScope: scope,
		})

		if err != nil {
			return err
		}
		if generateAppPasswordResponse.Status.Code != rpc.Code_CODE_OK {
			return formatError(generateAppPasswordResponse.Status)
		}

		err = printTableAppPasswords([]*authapp.AppPassword{generateAppPasswordResponse.AppPassword})
		if err != nil {
			return err
		}

		return nil
	}

	return cmd
}

func getScope(ctx context.Context, client gateway.GatewayAPIClient, opts *appTokenCreateOpts) (map[string]*authpb.Scope, error) {
	if opts.Unlimited {
		return scope.AddOwnerScope(nil)
	}

	var scopes map[string]*authpb.Scope
	var err error
	if len(opts.Share) != 0 {
		// TODO(gmgigi96): verify format
		for _, entry := range opts.Share {
			// share = xxxx:[r|w]
			shareIDPerm := strings.Split(entry, ":")
			shareID, perm := shareIDPerm[0], shareIDPerm[1]
			scopes, err = getPublicShareScope(ctx, client, shareID, perm, scopes)
			if err != nil {
				return nil, err
			}
		}
	}

	if len(opts.Path) != 0 {
		// TODO(gmgigi96): verify format
		for _, entry := range opts.Path {
			// path = /home/a/b:[r|w]
			pathPerm := strings.Split(entry, ":")
			path, perm := pathPerm[0], pathPerm[1]
			scopes, err = getPathScope(ctx, client, path, perm, scopes)
			if err != nil {
				return nil, err
			}
		}
	}

	return scopes, nil
}

func getPublicShareScope(ctx context.Context, client gateway.GatewayAPIClient, shareID, perm string, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
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

	return scope.AddPublicShareScope(publicShareResponse.GetShare(), role, scopes)
}

func getPathScope(ctx context.Context, client gateway.GatewayAPIClient, path, perm string, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	role, err := parsePermission(perm)
	if err != nil {
		return nil, err
	}

	statResponse, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: path}})

	if err != nil {
		return nil, err
	}
	if statResponse.Status.Code != rpc.Code_CODE_OK {
		return nil, formatError(statResponse.Status)
	}

	return scope.AddResourceInfoScope(statResponse.GetInfo(), role, scopes)
}

// parse permission string in the form of "rw" to create a role.
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

func checkOpts(opts *appTokenCreateOpts) error {
	if len(opts.Share) == 0 && len(opts.Path) == 0 && !opts.Unlimited {
		return errtypes.BadRequest("specify a token scope")
	}
	return nil
}
