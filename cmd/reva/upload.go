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
	"os"
	"path/filepath"
	"strconv"

	"github.com/cheggaaa/pb"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/eventials/go-tus"
	"github.com/eventials/go-tus/memorystore"

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

		gwc, err := getClient()
		if err != nil {
			return err
		}

		req := &provider.InitiateFileUploadRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: target,
				},
			},
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"Upload-Length": {
						Decoder: "plain",
						Value:   []byte(strconv.FormatInt(md.Size(), 10)),
					},
				},
			},
		}

		res, err := gwc.InitiateFileUpload(ctx, req)
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

		// create the tus client.
		c := tus.DefaultConfig()
		c.Resume = true
		c.HttpClient = rhttp.GetHTTPClient(ctx)
		c.Store, err = memorystore.NewMemoryStore()
		if err != nil {
			return err
		}
		if res.Token != "" {
			fmt.Printf("using X-Reva-Transfer header\n")
			c.Header.Add("X-Reva-Transfer", res.Token)
		} else if token, ok := tokenpkg.ContextGetToken(ctx); ok {
			fmt.Printf("using %s header\n", tokenpkg.TokenHeader)
			c.Header.Add(tokenpkg.TokenHeader, token)
		}
		tusc, err := tus.NewClient(dataServerURL, c)
		if err != nil {
			return err
		}

		metadata := map[string]string{
			"filename": filepath.Base(target),
			"dir":      filepath.Dir(target),
			"checksum": fmt.Sprintf("%s %s", storageprovider.GRPC2PKGXS(xsType).String(), xs),
		}

		fingerprint := fmt.Sprintf("%s-%d-%s-%s", md.Name(), md.Size(), md.ModTime(), xs)

		bar := pb.New(int(md.Size())).SetUnits(pb.U_BYTES)
		bar.Start()
		reader := bar.NewProxyReader(fd)

		// create an upload from a file.
		upload := tus.NewUpload(reader, md.Size(), metadata, fingerprint)

		// create the uploader.
		c.Store.Set(upload.Fingerprint, dataServerURL)
		uploader := tus.NewUploader(tusc, dataServerURL, upload, 0)

		// start the uploading process.
		err = uploader.Upload()
		if err != nil {
			return err
		}

		bar.Finish()

		req2 := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: target,
				},
			},
		}
		res2, err := gwc.Stat(ctx, req2)
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
