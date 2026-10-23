// Copyright 2018-2026 CERN
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
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/cheggaaa/pb"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/cs3org/reva/v3/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/crypto"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/studio-b12/gowebdav"
	"golang.org/x/term"
)

func uploadCommand(rt *cli.Runtime) *cobra.Command {
	var xsFlag, protocol string
	cmd := &cobra.Command{
		Use:   "upload <local_file> <remote_path>",
		Short: "Upload a local file",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return nil, cobra.ShellCompDirectiveDefault
			case 1:
				return listRemote(cmd, rt, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			target := args[1]
			absPath, err := utils.ResolvePath(args[0])
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
			fmt.Fprintf(out, "Local file size: %d bytes\n", md.Size())

			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
				Ref: &provider.Reference{Path: target},
				Opaque: &typespb.Opaque{
					Map: map[string]*typespb.OpaqueEntry{
						"Upload-Length": {Decoder: "plain", Value: []byte(strconv.FormatInt(md.Size(), 10))},
					},
				},
			})
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
				fmt.Fprintln(out, "File uploaded")
				return nil
			}

			if protocol != "simple" {
				return errors.New("upload: protocol not supported: " + protocol)
			}
			p, err := getUploadProtocolInfo(res.Protocols, protocol)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Data server: %s\n", p.UploadEndpoint)
			fmt.Fprintf(out, "Allowed checksums: %+v\n", p.AvailableChecksums)

			xsType, err := guessXS(xsFlag, p.AvailableChecksums)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Checksum selected: %s\n", xsType)
			xs, err := computeXS(xsType, fd)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Local XS: %s:%s\n", xsType, xs)
			if _, err := fd.Seek(0, io.SeekStart); err != nil {
				return err
			}

			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, p.UploadEndpoint, fd)
			if err != nil {
				return err
			}
			httpReq.Header.Set(datagateway.TokenTransportHeader, p.Token)
			httpReq.Header.Set("Upload-Length", strconv.FormatInt(md.Size(), 10))
			q := httpReq.URL.Query()
			q.Add("xs", xs)
			q.Add("xs_type", storageprovider.GRPC2PKGXS(xsType).String())
			httpReq.URL.RawQuery = q.Encode()
			httpRes, err := rt.HTTPClient().Do(httpReq)
			if err != nil {
				return err
			}
			defer httpRes.Body.Close()
			if httpRes.StatusCode != http.StatusOK {
				return errors.New("upload: PUT request returned " + httpRes.Status)
			}

			res2, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: target}})
			if err != nil {
				return err
			}
			if res2.Status.Code != rpc.Code_CODE_OK {
				return formatError(res2.Status)
			}
			info := res2.Info
			fmt.Fprintf(out, "File uploaded: %s:%s %d %s\n", info.Id.StorageId, info.Id.OpaqueId, info.Size, info.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&xsFlag, "xs", "negotiate", "checksum type, or negotiate to use the server preference")
	cmd.Flags().StringVar(&protocol, "protocol", "simple", "upload protocol")
	return cmd
}

func downloadCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "download <remote_file> <local_file>",
		Short: "Download a file to the local file system",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return listRemote(cmd, rt, toComplete)
			case 1:
				return nil, cobra.ShellCompDirectiveDefault
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, local := args[0], args[1]
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res1, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: remote}})
			if err != nil {
				return err
			}
			if res1.Status.Code != rpc.Code_CODE_OK {
				return formatError(res1.Status)
			}
			info := res1.Info

			res, err := client.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{Ref: &provider.Reference{Path: remote}})
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
			fmt.Fprintf(cmd.OutOrStdout(), "Downloading from: %s\n", p.DownloadEndpoint)

			content, err := checkDownloadWebdavRef(res.Protocols)
			if err != nil {
				if _, ok := err.(errtypes.IsNotSupported); !ok {
					return err
				}
				httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.DownloadEndpoint, nil)
				if err != nil {
					return err
				}
				httpReq.Header.Set(datagateway.TokenTransportHeader, p.Token)
				httpRes, err := rt.HTTPClient().Do(httpReq)
				if err != nil {
					return err
				}
				defer httpRes.Body.Close()
				if httpRes.StatusCode != http.StatusOK {
					return errors.New("download: GET request returned " + httpRes.Status)
				}
				content = httpRes.Body
			}

			absPath, err := utils.ResolvePath(local)
			if err != nil {
				return err
			}
			fd, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			// Show progress only to a person, not in logs or pipes.
			if term.IsTerminal(int(os.Stderr.Fd())) {
				bar := pb.New(int(info.Size)).SetUnits(pb.U_BYTES)
				bar.Output = os.Stderr
				bar.Start()
				defer bar.Finish()
				content = bar.NewProxyReader(content)
			}
			if _, err := io.Copy(fd, content); err != nil {
				fd.Close()
				return err
			}
			return fd.Close()
		},
	}
}

func getUploadProtocolInfo(protocolInfos []*gateway.FileUploadProtocol, protocol string) (*gateway.FileUploadProtocol, error) {
	for _, p := range protocolInfos {
		if p.Protocol == protocol {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(protocol)
}

func getDownloadProtocolInfo(protocolInfos []*gateway.FileDownloadProtocol, protocol string) (*gateway.FileDownloadProtocol, error) {
	for _, p := range protocolInfos {
		if p.Protocol == protocol {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(protocol)
}

// webdavRef returns the WebDAV token and file path that some storage
// providers put in the opaque map of the simple protocol.
func webdavRef(o *typespb.Opaque) (token, filePath string, err error) {
	if o == nil {
		return "", "", errtypes.NotSupported("opaque object not defined")
	}
	for key, dst := range map[string]*string{"webdav-token": &token, "webdav-file-path": &filePath} {
		e, ok := o.Map[key]
		if !ok {
			return "", "", errtypes.NotSupported(key + " not defined")
		}
		if e.Decoder != "plain" {
			return "", "", errors.New("opaque entry decoder not recognized: " + e.Decoder)
		}
		*dst = string(e.Value)
	}
	return token, filePath, nil
}

func checkUploadWebdavRef(protocols []*gateway.FileUploadProtocol, md os.FileInfo, fd *os.File) error {
	p, err := getUploadProtocolInfo(protocols, "simple")
	if err != nil {
		return err
	}
	token, filePath, err := webdavRef(p.Opaque)
	if err != nil {
		return err
	}
	c := gowebdav.NewClient(p.UploadEndpoint, "", "")
	c.SetHeader(appctx.TokenHeader, token)
	c.SetHeader("Upload-Length", strconv.FormatInt(md.Size(), 10))
	return c.WriteStream(filePath, fd, 0o700)
}

func checkDownloadWebdavRef(protocols []*gateway.FileDownloadProtocol) (io.Reader, error) {
	p, err := getDownloadProtocolInfo(protocols, "simple")
	if err != nil {
		return nil, err
	}
	token, filePath, err := webdavRef(p.Opaque)
	if err != nil {
		return nil, err
	}
	c := gowebdav.NewClient(p.DownloadEndpoint, "", "")
	c.SetHeader(appctx.TokenHeader, token)
	return c.ReadStream(filePath)
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
	invalid := provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID
	if xsFlag != "negotiate" {
		wanted := storageprovider.PKG2GRPCXS(xsFlag)
		if wanted == invalid {
			return invalid, fmt.Errorf("desired checksum is invalid: %s", xsFlag)
		}
		for _, xs := range availableXS {
			if xs.Type == wanted {
				return wanted, nil
			}
		}
		return invalid, fmt.Errorf("checksum not available server side: %s", xsFlag)
	}
	if len(availableXS) == 0 {
		return invalid, errors.New("no available xs for negotiating")
	}
	// A lower number is a higher priority.
	var chosen provider.ResourceChecksumType
	var maxPrio uint32 = math.MaxUint32
	for _, xs := range availableXS {
		if xs.Priority < maxPrio {
			maxPrio = xs.Priority
			chosen = xs.Type
		}
	}
	return chosen, nil
}
