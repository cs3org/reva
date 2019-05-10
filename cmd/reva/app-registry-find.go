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
	"mime"
	"os"
	"path"

	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
)

func appRegistryFindCommand() *command {
	cmd := newCommand("app-registry-find")
	cmd.Description = func() string {
		return "find applicaton provider for file extension or mimetype"
	}
	cmd.Action = func() error {
		if cmd.NArg() == 0 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]
		ext := path.Ext(fn)
		mime := mime.TypeByExtension(ext)
		req := &appregistryv0alphapb.GetAppProviderRequest{
			MimeType: mime,
		}

		client, err := getAppRegistryClient()
		if err != nil {
			return err
		}
		ctx := getAuthContext()
		res, err := client.GetAppProvider(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("application provider can be found at %s\n", res.Provider.Address)
		return nil
	}
	return cmd
}
