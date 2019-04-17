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

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageregistry/v0alpha"
)

func brokerFindCommand() *command {
	cmd := newCommand("broker-find")
	cmd.Description = func() string {
		return "find storage provider for path"
	}
	cmd.Action = func() error {
		fn := "/"
		if cmd.NArg() >= 1 {
			fn = cmd.Args()[0]
		}

		req := &storageregistryv0alphapb.GetStorageProviderRequest{
			Ref: &storageproviderv0alphapb.Reference{Spec: &storageproviderv0alphapb.Reference_Path{Path: fn}},
		}
		client, err := getStorageBrokerClient()
		if err != nil {
			return err
		}
		ctx := getAuthContext()
		res, err := client.GetStorageProvider(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("resource can be found at %s\n", res.Provider.Address)
		return nil
	}
	return cmd
}
