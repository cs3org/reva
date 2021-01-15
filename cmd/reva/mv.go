// Copyright 2018-2021 CERN
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
	"io"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"
)

func moveCommand() *command {
	cmd := newCommand("mv")
	cmd.Description = func() string { return "moves/rename a file/folder" }
	cmd.Usage = func() string { return "Usage: mv [-flags] <source> <destination>" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 2 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		src := cmd.Args()[0]
		dst := cmd.Args()[1]

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		sourceRef := &provider.Reference{
			Spec: &provider.Reference_Path{Path: src},
		}
		targetRef := &provider.Reference{
			Spec: &provider.Reference_Path{Path: dst},
		}
		req := &provider.MoveRequest{Source: sourceRef, Destination: targetRef}
		res, err := client.Move(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		return nil
	}
	return cmd
}
