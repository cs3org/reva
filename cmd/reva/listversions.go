// Copyright 2018-2024 CERN
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
	"io"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"
)

func listVersionsCommand() *command {
	cmd := newCommand("listversions")
	cmd.Description = func() string { return "list the versions of a file" }
	cmd.Usage = func() string { return "Usage: listversions <file_name>" }

	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		fn := cmd.Args()[0]
		client, err := getClient()
		if err != nil {
			return err
		}

		ref := &provider.Reference{Path: fn}
		req := &provider.ListFileVersionsRequest{Ref: ref}

		ctx := getAuthContext()
		res, err := client.ListFileVersions(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		vers := res.Versions
		for _, info := range vers {
			fmt.Printf("Key: %s Size: %d mtime:%d\n", info.Key, info.Size, info.Mtime)
		}

		return nil
	}
	return cmd
}
