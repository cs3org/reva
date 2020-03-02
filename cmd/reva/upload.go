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
	"math"
	"net/http"
	"os"

	"github.com/cheggaaa/pb"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	// TODO(labkode): this should not come from this package.
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/pkg/crypto"
	"github.com/cs3org/reva/pkg/rhttp"
)

func uploadCommand() *command {
	cmd := newCommand("upload")
	cmd.Description = func() string { return "upload a local file to the remote server" }
	cmd.Usage = func() string { return "Usage: upload [-flags] <file_name> <remote_target>" }
	xsFlag := cmd.String("xs", "negotiate", "compute checksum")
	cmd.Action = func() error {
		ctx := getAuthContext()

		if cmd.NArg() < 2 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]
		target := cmd.Args()[1]

		fd, err := os.Open(fn)
		if err != nil {
			return err
		}
		defer fd.Close()

		md, err := fd.Stat()
		if err != nil {
			return err
		}
		defer fd.Close()

		fmt.Printf("Local file size: %d bytes\n", md.Size())

		client, err := getClient()
		if err != nil {
			return err
		}

		req := &provider.InitiateFileUploadRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: target,
				},
			},
		}

		res, err := client.InitiateFileUpload(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		// TODO(labkode): upload to data server
		fmt.Printf("Data server: %s\n", res.UploadEndpoint)
		fmt.Printf("Allowed checksums: %+v\n", res.AvailableChecksums)

		xsType, err := guessXS(*xsFlag, res.AvailableChecksums)
		if err != nil {
			return err
		}
		fmt.Printf("Checksum selected: %s\n", xsType)

		xs, err := computeXS(xsType, fd)
		if err != nil {
			return err
		}

		fmt.Printf("Local XS: %s:%s\n", xsType, xs)
		// seek back reader to 0
		if _, err := fd.Seek(0, 0); err != nil {
			return err
		}

		dataServerURL := res.UploadEndpoint
		bar := pb.New(int(md.Size())).SetUnits(pb.U_BYTES)
		bar.Start()
		reader := bar.NewProxyReader(fd)

		httpReq, err := rhttp.NewRequest(ctx, "PUT", dataServerURL, reader)
		if err != nil {
			return err
		}

		httpReq.Header.Set("X-Reva-Transfer", res.Token)
		q := httpReq.URL.Query()
		q.Add("xs", xs)
		q.Add("xs_type", storageprovider.GRPC2PKGXS(xsType).String())
		httpReq.URL.RawQuery = q.Encode()

		httpClient := rhttp.GetHTTPClient(ctx)

		httpRes, err := httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRes.Body.Close()

		bar.Finish()

		if httpRes.StatusCode != http.StatusOK {
			return err
		}

		req2 := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: target,
				},
			},
		}
		res2, err := client.Stat(ctx, req2)
		if err != nil {
			return err
		}

		if res2.Status.Code != rpc.Code_CODE_OK {
			return formatError(res2.Status)
		}

		info := res2.Info

		fmt.Printf("File uploaded: %s:%s %d %s\n", info.Id.StorageId, info.Id.OpaqueId, info.Size, info.Path)

		return nil
	}
	return cmd
}

func computeXS(t provider.ResourceChecksumType, r io.Reader) (string, error) {
	switch t {
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32:
		return crypto.ComputeAdler32XS(r)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5:
		return crypto.ComputeMD5XS(r)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1:
		return crypto.ComputeSHA1XS(r)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET:
		return "", nil
	default:
		return "", fmt.Errorf("invalid checksum: %s", t)

	}
}

func guessXS(xsFlag string, availableXS []*provider.ResourceChecksumPriority) (provider.ResourceChecksumType, error) {
	// force use of cheksum if available server side.
	if xsFlag != "negotiate" {
		wanted := storageprovider.PKG2GRPCXS(xsFlag)
		if wanted == provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID {
			return provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, fmt.Errorf("desired checksum is invalid: %s", xsFlag)
		}
		if isXSAvailable(wanted, availableXS) {
			return wanted, nil
		}
		return provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, fmt.Errorf("checksum not available server side: %s", xsFlag)
	}

	// negotiate the checksum type based on priority list from server-side.
	if len(availableXS) == 0 {
		return provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, fmt.Errorf("no available xs for negotiating")
	}

	// pick the one with priority to the lower number.
	desired := pickChecksumWithHighestPrio(availableXS)
	return desired, nil
}

func pickChecksumWithHighestPrio(xss []*provider.ResourceChecksumPriority) provider.ResourceChecksumType {
	var chosen provider.ResourceChecksumType
	var maxPrio uint32 = math.MaxUint32
	for _, xs := range xss {
		if xs.Priority < maxPrio {
			maxPrio = xs.Priority
			chosen = xs.Type
		}
	}
	return chosen
}

func isXSAvailable(t provider.ResourceChecksumType, available []*provider.ResourceChecksumPriority) bool {
	for _, xsPrio := range available {
		if xsPrio.Type == t {
			return true
		}
	}
	return false
}
