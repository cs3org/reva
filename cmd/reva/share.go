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

package main

import (
	"fmt"
	"os"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/pkg/errors"
)

func shareCommand() *command {
	cmd := newCommand("share")
	cmd.Description = func() string { return "share module" }
	cmd.Usage = func() string { return "Usage: share <create|remove|list|update>" }
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]

		ctx := getAuthContext()
		client, err := getStorageProviderClient()
		if err != nil {
			return err
		}

		ref := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		}
		req := &storageproviderv0alphapb.CreateContainerRequest{Ref: ref}
		res, err := client.CreateContainer(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		return nil
	}
	return cmd
}

func shareCreateCommmand() *command {
	cmd := newCommand("create")
	cmd.Description = func() string { return "create share" }
	cmd.Usage = func() string { return "Usage: share create [-flags] <path>" }
	grantType := cmd.String("t", "user", "grantee type (user or group)")
	grantee := cmd.String("g", "", "the grantee")
	idp := cmd.String("idp", "", "the idp of the grantee, default to same idp as the user triggering the action")
	rol := cmd.String("r", "viewer", "the permission for the share (viewer or editor)")
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[1]

		ctx := getAuthContext()
		client, err := getStorageProviderClient()
		if err != nil {
			return err
		}

		ref := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		}

		req := &storageproviderv0alphapb.StatRequest{Ref: ref}
		res, err := client.Stat(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		shareClient, err := getUserShareProviderClient()
		if err != nil {
			return err
		}

		perm, err := getSharePerm(*rol)
		if err != nil {
			return err
		}

		gt := getGrantType(*grantType)

		grant := &usershareproviderv0alphapb.ShareGrant{
			Permissions: perm,
			Grantee: &storageproviderv0alphapb.Grantee{
				Type: gt,
				Id: &typespb.UserId{
					Idp:      *idp,
					OpaqueId: *grantee,
				},
			},
		}
		shareRequest := &usershareproviderv0alphapb.CreateShareRequest{
			ResourceInfo: res.Info,
			Grant:        grant,
		}

		shareRes, err := shareClient.CreateShare(ctx, shareRequest)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Println(shareRes.Share.String())
		return nil
	}
	return cmd
}

func getGrantType(t string) storageproviderv0alphapb.GranteeType {
	switch t {
	case "u":
		return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER
	case "g":
		return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP
	default:
		return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID
	}
}

func getSharePerm(p string) (*usershareproviderv0alphapb.SharePermissions, error) {
	if p == "viewer" {
		return &usershareproviderv0alphapb.SharePermissions{
			Permissions: &storageproviderv0alphapb.ResourcePermissions{
				GetPath:              true,
				InitiateFileDownload: true,
				ListFileVersions:     true,
				ListContainer:        true,
				Stat:                 true,
			},
		}, nil
	} else if p == "editor" {
		return &usershareproviderv0alphapb.SharePermissions{
			Permissions: &storageproviderv0alphapb.ResourcePermissions{
				GetPath:              true,
				InitiateFileDownload: true,
				ListFileVersions:     true,
				ListContainer:        true,
				Stat:                 true,
				CreateContainer:      true,
				Delete:               true,
				InitiateFileUpload:   true,
				RestoreFileVersion:   true,
				Move:                 true,
			},
		}, nil
	}
	return nil, errors.New("invalid rol: " + p)
}
