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
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/cheggaaa/pb"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
	"github.com/studio-b12/gowebdav"
)

func downloadCommand() *command {
	cmd := newCommand("download")
	cmd.Description = func() string { return "download a remote file to the local filesystem" }
	cmd.Usage = func() string { return "Usage: download [-flags] <remote_file> <local_file>" }
	cmd.Action = func(w ...io.Writer) error {
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

		p, err := getDownloadProtocolInfo(res.Protocols, "simple")
		if err != nil {
			return err
		}

		// TODO(labkode): upload to data server
		fmt.Printf("Downloading from: %s\n", p.DownloadEndpoint)

		content, err := checkDownloadWebdavRef(res.Protocols)
		if err != nil {
			if _, ok := err.(errtypes.IsNotSupported); !ok {
				return err
			}

			dataServerURL := p.DownloadEndpoint
			// TODO(labkode): do a protocol switch
			httpReq, err := rhttp.NewRequest(ctx, "GET", dataServerURL, nil)
			if err != nil {
				return err
			}

			httpReq.Header.Set(datagateway.TokenTransportHeader, p.Token)
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
			content = httpRes.Body
		}

		absPath, err := utils.ResolvePath(local)
		if err != nil {
			return err
		}

		bar := pb.New(int(info.Size)).SetUnits(pb.U_BYTES)
		bar.Start()
		reader := bar.NewProxyReader(content)

		fd, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(fd, reader); err != nil {
			return err
		}
		bar.Finish()
		return nil
	}
	return cmd
}

func getDownloadProtocolInfo(protocolInfos []*gateway.FileDownloadProtocol, protocol string) (*gateway.FileDownloadProtocol, error) {
	for _, p := range protocolInfos {
		if p.Protocol == protocol {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(protocol)
}

func checkDownloadWebdavRef(protocols []*gateway.FileDownloadProtocol) (io.Reader, error) {
	p, err := getDownloadProtocolInfo(protocols, "simple")
	if err != nil {
		return nil, err
	}

	if p.Opaque == nil {
		return nil, errtypes.NotSupported("opaque object not defined")
	}

	var token string
	tokenOpaque, ok := p.Opaque.Map["webdav-token"]
	if !ok {
		return nil, errtypes.NotSupported("webdav token not defined")
	}
	switch tokenOpaque.Decoder {
	case "plain":
		token = string(tokenOpaque.Value)
	default:
		return nil, errors.New("opaque entry decoder not recognized: " + tokenOpaque.Decoder)
	}

	var filePath string
	fileOpaque, ok := p.Opaque.Map["webdav-file-path"]
	if !ok {
		return nil, errtypes.NotSupported("webdav file path not defined")
	}
	switch fileOpaque.Decoder {
	case "plain":
		filePath = string(fileOpaque.Value)
	default:
		return nil, errors.New("opaque entry decoder not recognized: " + fileOpaque.Decoder)
	}

	c := gowebdav.NewClient(p.DownloadEndpoint, "", "")
	c.SetHeader(tokenpkg.TokenHeader, token)

	reader, err := c.ReadStream(filePath)
	if err != nil {
		return nil, err
	}
	return reader, nil
}
