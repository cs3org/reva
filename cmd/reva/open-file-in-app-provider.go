// Copyright 2018-2020 CERN
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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"
)

func openFileInAppProviderCommand() *command {
	cmd := newCommand("open-file-in-app-provider")
	cmd.Description = func() string { return "open a file in an external app provider" }
	cmd.Usage = func() string {
		return "Usage: open-file-in-app-provider [-flags] [-viewmode view|read|write] <path>"
	}
	viewMode := cmd.String("viewmode", "view", "the view permissions, defaults to view")

	cmd.ResetFlags = func() {
		*viewMode = "view"
	}

	cmd.Action = func(w ...io.Writer) error {
		ctx := getAuthContext()
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}
		path := cmd.Args()[0]

		vm := getViewMode(*viewMode)

		client, err := getClient()
		if err != nil {
			return err
		}

		ref := &provider.Reference{
			Spec: &provider.Reference_Path{Path: path},
		}

		openRequest := &gateway.OpenFileInAppProviderRequest{Ref: ref, ViewMode: vm}

		openRes, err := client.OpenFileInAppProvider(ctx, openRequest)
		if err != nil {
			return err
		}

		if openRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(openRes.Status)
		}

		fmt.Println("App provider url: " + openRes.AppProviderUrl)

		return nil
	}
	return cmd
}

func getViewMode(viewMode string) gateway.OpenFileInAppProviderRequest_ViewMode {
	switch viewMode {
	case "view":
		return gateway.OpenFileInAppProviderRequest_VIEW_MODE_VIEW_ONLY
	case "read":
		return gateway.OpenFileInAppProviderRequest_VIEW_MODE_READ_ONLY
	case "write":
		return gateway.OpenFileInAppProviderRequest_VIEW_MODE_READ_WRITE
	default:
		return gateway.OpenFileInAppProviderRequest_VIEW_MODE_INVALID
	}
}
