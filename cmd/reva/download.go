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
	"net/http"
	"os"
	"time"

	"github.com/cheggaaa/pb"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/cmd/reva/command"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
)

func downloadCommand() *command.Command {
	cmd := command.NewCommand("download")
	cmd.Description = func() string { return "download a remote file into the local filesystem" }
	cmd.Usage = func() string { return "Usage: download [-flags] <remote_file> <local_file>" }
	cmd.Action = func() error {
		if cmd.NArg() < 2 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		remote := cmd.Args()[0]
		local := cmd.Args()[1]

		client, err := getClient()
		if err != nil {
			return err
		}

		ref := &provider.Reference{
			Spec: &provider.Reference_Path{Path: remote},
		}
		req1 := &provider.StatRequest{Ref: ref}
		ctx := getAuthContext()
		res1, err := client.Stat(ctx, req1)
		if err != nil {
			return err
		}
		if res1.Status.Code != rpc.Code_CODE_OK {
			return formatError(res1.Status)
		}

		info := res1.Info

		req2 := &provider.InitiateFileDownloadRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: remote,
				},
			},
		}
		res, err := client.InitiateFileDownload(ctx, req2)
		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		// TODO(labkode): upload to data server
		fmt.Printf("Downloading from: %s\n", res.DownloadEndpoint)

		dataServerURL := res.DownloadEndpoint
		// TODO(labkode): do a protocol switch
		httpReq, err := rhttp.NewRequest(ctx, "GET", dataServerURL, nil)
		if err != nil {
			return err
		}

		httpReq.Header.Set(datagateway.TokenTransportHeader, res.Token)
		httpClient := rhttp.GetHTTPClient(
			rhttp.Context(ctx),
			// TODO make insecure configurable
			rhttp.Insecure(true),
			// TODO make timeout configurable
			rhttp.Timeout(time.Duration(24*int64(time.Hour))),
		)

		httpRes, err := httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRes.Body.Close()

		if httpRes.StatusCode != http.StatusOK {
			return err
		}

		absPath, err := utils.ResolvePath(local)
		if err != nil {
			return err
		}

		fd, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}

		bar := pb.New(int(info.Size)).SetUnits(pb.U_BYTES)
		bar.Start()
		reader := bar.NewProxyReader(httpRes.Body)
		if _, err := io.Copy(fd, reader); err != nil {
			return err
		}
		bar.Finish()
		return nil

	}
	return cmd
}
