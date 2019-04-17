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
	"context"
	"fmt"
	"os"

	appproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/appprovider/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func appProviderGetIFrameCommand() *command {
	cmd := newCommand("app-provider-get-iframe")
	cmd.Description = func() string {
		return "find iframe UI provider for filename"
	}
	cmd.Action = func() error {
		if cmd.NArg() < 3 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		// TODO(labkode): contact first storage provider to get metadata for the resource
		// and then get the resource id.
		appProvider := cmd.Args()[0]
		fn := cmd.Args()[1]
		token := cmd.Args()[2]
		req := &appproviderv0alphapb.OpenRequest{
			ResourceId:  &storageproviderv0alphapb.ResourceId{OpaqueId: fn}, // TODO(labkode): fix me
			AccessToken: token,
		}

		client, err := getAppProviderClient(appProvider)
		if err != nil {
			return err
		}
		ctx := context.Background()
		res, err := client.Open(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("Load in your browser the following iframe to edit the resource: %s", res.IframeUrl)
		return nil
	}
	return cmd
}
