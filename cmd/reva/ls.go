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

	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func lsCommand() *command {
	cmd := newCommand("ls")
	cmd.Description = func() string { return "list a folder contents" }
	longFlag := cmd.Bool("l", false, "long listing")
	cmd.Action = func() error {
		if cmd.NArg() < 2 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		provider := cmd.Args()[0]
		fn := cmd.Args()[1]
		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		ref := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		}
		req := &storageproviderv0alphapb.ListContainerRequest{Ref: ref}

		ctx := getAuthContext()
		res, err := client.ListContainer(ctx, req)
		if err != nil {
			return err
		}

		infos := res.Infos
		for _, info := range infos {
			if *longFlag {
				fmt.Printf("%+v %d %d %v %s\n", info.PermissionSet, info.Mtime, info.Size, info.Id, info.Path)
			} else {
				fmt.Println(info.Path)
			}
		}
		return nil
	}
	return cmd
}
