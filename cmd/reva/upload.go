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
	"math"
	"net/http"
	"os"
	"strconv"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	// TODO(labkode): this should not come from this package.
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/internal/http/services/datagateway"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/crypto"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
	"github.com/studio-b12/gowebdav"
)

func uploadCommand() *command {
	cmd := newCommand("upload")
	cmd.Description = func() string { return "upload a local file to the remote server" }
	cmd.Usage = func() string { return "Usage: upload [-flags] <file_name> <remote_target>" }
	xsFlag := cmd.String("xs", "negotiate", "compute checksum")
	protocolFlag := cmd.String("protocol", "simple", "protocol for file uploads: simple, negotiate")

	cmd.ResetFlags = func() {
		*protocolFlag, *xsFlag = "simple", "negotiate"
	}

	cmd.Action = func(w ...io.Writer) error {
		ctx := getAuthContext()

		if cmd.NArg() < 2 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		fn := cmd.Args()[0]
		target := cmd.Args()[1]

		absPath, err := utils.ResolvePath(fn)
		if err != nil {
			return err
		}

		fd, err := os.Open(absPath)
		if err != nil {
			return err
		}
		defer fd.Close()

		md, err := fd.Stat()
		if err != nil {
			return err
		}

		fmt.Printf("Local file size: %d bytes\n", md.Size())

		gwc, err := getClient()
		if err != nil {
			return err
		}

		req := &provider.InitiateFileUploadRequest{
			Ref: &provider.Reference{Path: target},
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

		if err = checkUploadWebdavRef(res.Protocols, md, fd); err != nil {
			if _, ok := err.(errtypes.IsNotSupported); !ok {
				return err
			}
		} else {
			return nil
		}

		p, err := getUploadProtocolInfo(res.Protocols, *protocolFlag)
		if err != nil {
			return err
		}

		fmt.Printf("Data server: %s\n", p.UploadEndpoint)
		fmt.Printf("Allowed checksums: %+v\n", p.AvailableChecksums)

		xsType, err := guessXS(*xsFlag, p.AvailableChecksums)
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

		dataServerURL := p.UploadEndpoint

		if *protocolFlag == "simple" {
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, dataServerURL, fd)
			if err != nil {
				return err
			}

			httpReq.Header.Set(datagateway.TokenTransportHeader, p.Token)
			q := httpReq.URL.Query()
			q.Add("xs", xs)
			q.Add("xs_type", storageprovider.GRPC2PKGXS(xsType).String())
			httpReq.URL.RawQuery = q.Encode()

			httpRes, err := client.Do(httpReq)
			if err != nil {
				return err
			}
			defer httpRes.Body.Close()
			if httpRes.StatusCode != http.StatusOK {
				return errors.New("upload: PUT request returned " + httpRes.Status)
			}
		} else {
			return errors.New("upload: protocol not supported: " + *protocolFlag)
		}

		req2 := &provider.StatRequest{
			Ref: &provider.Reference{Path: target},
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

func getUploadProtocolInfo(protocolInfos []*gateway.FileUploadProtocol, protocol string) (*gateway.FileUploadProtocol, error) {
	for _, p := range protocolInfos {
		if p.Protocol == protocol {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(protocol)
}

func checkUploadWebdavRef(protocols []*gateway.FileUploadProtocol, md os.FileInfo, fd *os.File) error {
	p, err := getUploadProtocolInfo(protocols, "simple")
	if err != nil {
		return err
	}

	if p.Opaque == nil {
		return errtypes.NotSupported("opaque object not defined")
	}

	var token string
	tokenOpaque, ok := p.Opaque.Map["webdav-token"]
	if !ok {
		return errtypes.NotSupported("webdav token not defined")
	}
	switch tokenOpaque.Decoder {
	case "plain":
		token = string(tokenOpaque.Value)
	default:
		return errors.New("opaque entry decoder not recognized: " + tokenOpaque.Decoder)
	}

	var filePath string
	fileOpaque, ok := p.Opaque.Map["webdav-file-path"]
	if !ok {
		return errtypes.NotSupported("webdav file path not defined")
	}
	switch fileOpaque.Decoder {
	case "plain":
		filePath = string(fileOpaque.Value)
	default:
		return errors.New("opaque entry decoder not recognized: " + fileOpaque.Decoder)
	}

	c := gowebdav.NewClient(p.UploadEndpoint, "", "")
	c.SetHeader(appctx.TokenHeader, token)
	c.SetHeader("Upload-Length", strconv.FormatInt(md.Size(), 10))

	if err = c.WriteStream(filePath, fd, 0700); err != nil {
		return err
	}

	fmt.Println("File uploaded")
	return nil
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
	// force use of checksum if available server side.
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
