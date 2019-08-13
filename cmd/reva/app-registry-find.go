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

	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
)

func appRegistryFindCommand() *command {
	cmd := newCommand("app-registry-find")
	cmd.Description = func() string {
		return "find application provider for file extension or mimetype"
	}
	cmd.Usage = func() string { return "Usage: app-registry-find <file>" }
	cmd.Action = func() error {
		if cmd.NArg() == 0 {
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
		req := &storageproviderv0alphapb.StatRequest{Ref: ref}
		res, err := client.Stat(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		req2 := &appregistryv0alphapb.GetAppProvidersRequest{
			ResourceInfo: res.Info,
		}

		client2, err := getAppRegistryClient()
		if err != nil {
			return err
		}

		res2, err := client2.GetAppProviders(ctx, req2)
		if err != nil {
			return err
		}

		if res2.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res2.Status)
		}

		for _, p := range res2.Providers {
			fmt.Printf("%s\n", p.Address)
		}

		return nil
	}
	return cmd
}
