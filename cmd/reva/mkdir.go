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

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func mkdirCommand() *command {
	cmd := newCommand("mkdir")
	cmd.Description = func() string { return "creates a folder" }
	cmd.Action = func() error {
		if cmd.NArg() < 2 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]
		provider := cmd.Args()[1]

		ctx := getAuthContext()
		client, err := getStorageProviderClient(provider)
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
