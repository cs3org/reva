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
	"os"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/pkg/errors"
)

func openFileInAppProviderCommand() *command {
	cmd := newCommand("open-file-in-app-provider")
	cmd.Description = func() string { return "Open a file in an external app provider" }
	cmd.Usage = func() string {
		return "Usage: open-file-in-app-provider [-flags] <path> <viewMode (view, read, write)>"
	}
	viewMode := cmd.String("viewMode", "view", "the view permissions, defaults to view")

	cmd.Action = func() error {
		ctx := getAuthContext()
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}
		path := cmd.Args()[0]

		viewMode := getViewMode(*viewMode)

		client, err := getClient()
		if err != nil {
			return err
		}

		ref := &provider.Reference{
			Spec: &provider.Reference_Path{Path: path},
		}
		accessToken, ok := tokenpkg.ContextGetToken(ctx)
		if !ok || accessToken == "" {
			err := errors.New("Access token is invalid or empty")
			return err
		}

		openRequest := &providerpb.OpenFileInAppProviderRequest{Ref: ref, AccessToken: accessToken, ViewMode: viewMode}

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

func getViewMode(viewMode string) providerpb.OpenFileInAppProviderRequest_ViewMode {
	switch viewMode {
	case "view":
		return providerpb.OpenFileInAppProviderRequest_VIEW_MODE_VIEW_ONLY
	case "read":
		return providerpb.OpenFileInAppProviderRequest_VIEW_MODE_READ_ONLY
	case "write":
		return providerpb.OpenFileInAppProviderRequest_VIEW_MODE_READ_WRITE
	default:
		return providerpb.OpenFileInAppProviderRequest_VIEW_MODE_INVALID
	}
}
