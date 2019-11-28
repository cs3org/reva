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
	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
)

func rmCommand() *command {
	cmd := newCommand("rm")
	cmd.Description = func() string { return "removes a file or folder" }
	cmd.Usage = func() string { return "Usage: rm [-flags] <file_name>" }
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]
		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		ref := &storageproviderv1beta1pb.Reference{
			Spec: &storageproviderv1beta1pb.Reference_Path{Path: fn},
		}
		req := &storageproviderv1beta1pb.DeleteRequest{Ref: ref}
		res, err := client.Delete(ctx, req)
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
