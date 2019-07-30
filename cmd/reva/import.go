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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
)

func importCommand() *command {
	cmd := newCommand("import")
	cmd.Description = func() string { return "import metadata" }
	cmd.Usage = func() string { return "Usage: import [-flags] <user export folder>" }
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}
		f := cmd.Args()[0]

		sClient, err := getStorageProviderClient()
		if err != nil {
			return err
		}
		uClient, err := getUserShareProviderClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()

		// TODO read roles mapping

		// open files.jsonl
		// see https://github.com/owncloud/data_exporter/issues/77#issuecomment-507582067 for the import file layout
		// the concrete format still needs to be determined

		file, err := os.Open(path.Join(f, "files.jsonl"))
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		dec := json.NewDecoder(file)

		for {
			var j map[string]interface{}
			if err := dec.Decode(&j); err == io.EOF {
				break
			} else if err != nil {
				// TODO write to error jsonl and continue?
				log.Fatal(err)
				return err
			}
			fn := path.Join("/", path.Base(f), j["path"].(string))

			sReq := &storageproviderv0alphapb.StatRequest{
				Ref: &storageproviderv0alphapb.Reference{
					Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
				},
			}
			sRes, err := sClient.Stat(ctx, sReq)
			if err != nil {
				// TODO log that a file does not exist
				log.Fatal(err)
				return err
			}
			if sRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
				// TODO log that a file does not exist
				log.Print("file does not exist: " + fn)
				continue
			}

			info := sRes.Info

			if j["shares"] != nil {
				// update shares
				for _, s := range j["shares"].([]interface{}) {
					share := s.(map[string]interface{})
					role := share["role"].(string)
					permissions := &storageproviderv0alphapb.ResourcePermissions{}

					// TODO use roles mapping to map to permissions
					switch role {
					case "viewer":
						permissions.Stat = true
						permissions.ListContainer = true
						permissions.InitiateFileDownload = true

						permissions.ListGrants = true
					case "editor":
						permissions.Stat = true
						permissions.ListContainer = true
						permissions.InitiateFileDownload = true

						permissions.CreateContainer = true
						permissions.InitiateFileUpload = true
						permissions.Delete = true
						permissions.Move = true

						permissions.ListGrants = true
					case "co-owner":
						permissions.Stat = true
						permissions.ListContainer = true
						permissions.InitiateFileDownload = true

						permissions.CreateContainer = true
						permissions.InitiateFileUpload = true
						permissions.Delete = true
						permissions.Move = true

						permissions.ListGrants = true
						permissions.AddGrant = true
						permissions.RemoveGrant = true
						permissions.UpdateGrant = true
					}
					user := share["user"].(string)
					shareID := "u:" + user + "@" + info.GetId().OpaqueId
					uReq := &usershareproviderv0alphapb.UpdateShareRequest{
						Ref: &usershareproviderv0alphapb.ShareReference{
							Spec: &usershareproviderv0alphapb.ShareReference_Id{
								Id: &usershareproviderv0alphapb.ShareId{
									OpaqueId: shareID,
								},
							},
						},
						Field: &usershareproviderv0alphapb.UpdateShareRequest_UpdateField{
							Field: &usershareproviderv0alphapb.UpdateShareRequest_UpdateField_Permissions{
								Permissions: &usershareproviderv0alphapb.SharePermissions{
									// this completely overwrites the permissions for this user
									Permissions: permissions,
								},
							},
						},
					}
					_, err := uClient.UpdateShare(ctx, uReq)
					if err != nil {
						return err
					}
				}
			}

		}

		return nil
	}
	return cmd
}
