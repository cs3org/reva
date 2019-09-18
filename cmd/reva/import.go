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
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
)

// share representation in the import metadata
type share struct {
	Path           string `json:"path"`
	ShareType      string `json:"shareType"`
	Type           string `json:"type"`
	Owner          string `json:"owner"`
	SharedBy       string `json:"sharedBy"`
	SharedWith     string `json:"sharedWith"`
	Permissions    int    `json:"permissions"`
	ExpirationDate string `json:"expirationDate"`
	Password       string `json:"password"`
	Name           string `json:"name"`
	Token          string `json:"token"`
}

// Maps oc10 permissions to roles
var ocPermToRole = map[int]string{
	1:  "viewer",
	15: "co-owner",
	31: "editor",
}

func importCommand() *command {
	cmd := newCommand("import")
	cmd.Description = func() string { return "import metadata" }
	cmd.Usage = func() string { return "Usage: import [-flags] <user export folder>" }
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}
		exportPath := cmd.Args()[0]

		err := importShares(exportPath)
		if err != nil {
			log.Fatal(err)
		}

		return err
	}
	return cmd
}

//Imports shares from a shares.jsonl file in exportPath. The files must already be present on the storage
func importShares(exportPath string) error {
	storage, err := getStorageProviderClient()
	if err != nil {
		return err
	}
	sharing, err := getUserShareProviderClient()
	if err != nil {
		return err
	}
	ctx := getAuthContext()
	sharesJSONL, err := os.Open(path.Join(exportPath, "shares.jsonl"))
	if err != nil {
		return err
	}
	defer sharesJSONL.Close()
	jsonLines := bufio.NewScanner(sharesJSONL)

	for jsonLines.Scan() {
		var shareData share
		if err := json.Unmarshal(jsonLines.Bytes(), &shareData); err != nil {
			log.Fatal(err)
			return err
		}

		//Stat file, skip share creation if it does not exist on the target system
		resourcePath := path.Join("/", path.Base(exportPath), shareData.Path)
		statReq := &storageproviderv0alphapb.StatRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{Path: resourcePath},
			},
		}
		statResp, err := storage.Stat(ctx, statReq)

		if err != nil {
			log.Fatal(err)
		}

		if statResp.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			log.Print("File does not exist on target system, skipping share import: " + resourcePath)
			continue
		}

		_, err = sharing.CreateShare(ctx, shareReq(statResp.Info, &shareData))
		if err != nil {
			return err
		}
	}
	return nil
}

func shareReq(info *storageproviderv0alphapb.ResourceInfo, share *share) *usershareproviderv0alphapb.CreateShareRequest {
	return &usershareproviderv0alphapb.CreateShareRequest{
		ResourceInfo: info,
		Grant: &usershareproviderv0alphapb.ShareGrant{
			Grantee: &storageproviderv0alphapb.Grantee{
				Type: storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER,
				Id: &typespb.UserId{
					OpaqueId: share.SharedWith,
				},
			},
			Permissions: &usershareproviderv0alphapb.SharePermissions{
				Permissions: convertPermissions(share.Permissions),
			},
		},
	}
}

// Create resource permission-set from ownCloud permissions int
func convertPermissions(ocPermissions int) *storageproviderv0alphapb.ResourcePermissions {
	perms := &storageproviderv0alphapb.ResourcePermissions{}
	switch ocPermToRole[ocPermissions] {
	case "viewer":
		perms.Stat = true
		perms.ListContainer = true
		perms.InitiateFileDownload = true
		perms.ListGrants = true
	case "editor":
		perms.Stat = true
		perms.ListContainer = true
		perms.InitiateFileDownload = true

		perms.CreateContainer = true
		perms.InitiateFileUpload = true
		perms.Delete = true
		perms.Move = true
		perms.ListGrants = true
	case "co-owner":
		perms.Stat = true
		perms.ListContainer = true
		perms.InitiateFileDownload = true

		perms.CreateContainer = true
		perms.InitiateFileUpload = true
		perms.Delete = true
		perms.Move = true

		perms.ListGrants = true
		perms.AddGrant = true
		perms.RemoveGrant = true
		perms.UpdateGrant = true
	}

	return perms
}
