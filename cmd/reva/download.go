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
	"io"
	"net/http"
	"os"
	"time"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cheggaaa/pb"
)

func downloadCommand() *command {
	cmd := newCommand("download")
	cmd.Description = func() string { return "download a remote file into the local filesystem" }
	cmd.Action = func() error {
		if cmd.NArg() < 3 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		provider := cmd.Args()[0]
		remote := cmd.Args()[1]
		local := cmd.Args()[2]

		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		ref := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: remote},
		}
		req1 := &storageproviderv0alphapb.StatRequest{Ref: ref}
		ctx := getAuthContext()
		res1, err := client.Stat(ctx, req1)
		if err != nil {
			return err
		}
		if res1.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res1.Status)
		}

		info := res1.Info

		req2 := &storageproviderv0alphapb.InitiateFileDownloadRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{
					Path: remote,
				},
			},
		}
		res, err := client.InitiateFileDownload(ctx, req2)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		// TODO(labkode): upload to data server
		fmt.Printf("Downloading from: %s\n", res.DownloadEndpoint)

		dataServerURL := res.DownloadEndpoint
		// TODO(labkode): do a protocol switch
		httpReq, err := http.NewRequest("GET", dataServerURL, nil)
		if err != nil {
			return err
		}

		// TODO(labkode): harden http client
		// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
		httpClient := &http.Client{
			Timeout: time.Second * 10,
		}

		httpRes, err := httpClient.Do(httpReq)
		if err != nil {
			return err
		}

		if httpRes.StatusCode != http.StatusOK {
			return err
		}

		fd, err := os.OpenFile(local, os.O_CREATE|os.O_WRONLY, 0644)
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
