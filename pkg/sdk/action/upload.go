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

package action

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	p "path"
	"strconv"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storage "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/sdk"
	"github.com/cs3org/reva/pkg/sdk/common"
	"github.com/cs3org/reva/pkg/sdk/common/crypto"
	"github.com/cs3org/reva/pkg/sdk/common/net"
)

// UploadAction is used to upload files through Reva.
// WebDAV will be used automatically if the endpoint supports it. The EnableTUS flag specifies whether to use TUS if WebDAV is not supported.
type UploadAction struct {
	action

	EnableTUS bool
}

// UploadFile uploads the provided file to the target.
func (action *UploadAction) UploadFile(file *os.File, target string) (*storage.ResourceInfo, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("unable to stat the specified file: %v", err)
	}

	return action.upload(file, fileInfo, target)
}

// UploadFileTo uploads the provided file to the target directory, keeping the original file name.
func (action *UploadAction) UploadFileTo(file *os.File, path string) (*storage.ResourceInfo, error) {
	return action.UploadFile(file, p.Join(path, p.Base(file.Name())))
}

// UploadBytes uploads the provided byte data to the target.
func (action *UploadAction) UploadBytes(data []byte, target string) (*storage.ResourceInfo, error) {
	return action.Upload(bytes.NewReader(data), int64(len(data)), target)
}

// Upload uploads data from the provided reader to the target.
func (action *UploadAction) Upload(data io.Reader, size int64, target string) (*storage.ResourceInfo, error) {
	dataDesc := common.CreateDataDescriptor(p.Base(target), size)
	return action.upload(data, &dataDesc, target)
}

func (action *UploadAction) upload(data io.Reader, dataInfo os.FileInfo, target string) (*storage.ResourceInfo, error) {
	fileOpsAct := MustNewFileOperationsAction(action.session)

	dir := p.Dir(target)
	if err := fileOpsAct.MakePath(dir); err != nil {
		return nil, fmt.Errorf("unable to create target directory '%v': %v", dir, err)
	}

	// Issue a file upload request to Reva; this will provide the endpoint to write the file data to
	upload, err := action.initiateUpload(target, dataInfo.Size())
	if err != nil {
		return nil, err
	}

	simpleProtocol, err := getUploadProtocolInfo(upload.Protocols, "simple")
	if err != nil {
		return nil, err
	}

	// Try to upload the file via WebDAV first
	if client, values, err := net.NewWebDAVClientWithOpaque(simpleProtocol.UploadEndpoint, simpleProtocol.Opaque); err == nil {
		if err := client.Write(values[net.WebDAVPathName], data, dataInfo.Size()); err != nil {
			return nil, fmt.Errorf("error while writing to '%v' via WebDAV: %v", simpleProtocol.UploadEndpoint, err)
		}
	} else {
		// WebDAV is not supported, so directly write to the HTTP endpoint
		checksumType := action.selectChecksumType(simpleProtocol.AvailableChecksums)
		checksumTypeName := crypto.GetChecksumTypeName(checksumType)
		checksum, err := crypto.ComputeChecksum(checksumType, data)
		if err != nil {
			return nil, fmt.Errorf("unable to compute data checksum: %v", err)
		}

		// Check if the data object can be seeked; if so, reset it to its beginning
		if seeker, ok := data.(io.Seeker); ok {
			_, _ = seeker.Seek(0, 0)
		}

		if action.EnableTUS {
			tusProtocol, err := getUploadProtocolInfo(upload.Protocols, "tus")
			if err != nil {
				return nil, err
			}
			if err := action.uploadFileTUS(tusProtocol, target, data, dataInfo, checksum, checksumTypeName); err != nil {
				return nil, fmt.Errorf("error while writing to '%v' via TUS: %v", tusProtocol.UploadEndpoint, err)
			}
		} else if err := action.uploadFilePUT(simpleProtocol, data, checksum, checksumTypeName); err != nil {
			return nil, fmt.Errorf("error while writing to '%v' via HTTP: %v", simpleProtocol.UploadEndpoint, err)
		}
	}

	// Return information about the just-uploaded file
	return fileOpsAct.Stat(target)
}

func (action *UploadAction) initiateUpload(target string, size int64) (*gateway.InitiateFileUploadResponse, error) {
	// Initiating an upload request gets us the upload endpoint for the specified target
	req := &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: target,
			},
		},
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"Upload-Length": {
					Decoder: "plain",
					Value:   []byte(strconv.FormatInt(size, 10)),
				},
			},
		},
	}
	res, err := action.session.Client().InitiateFileUpload(action.session.Context(), req)
	if err := net.CheckRPCInvocation("initiating upload", res, err); err != nil {
		return nil, err
	}

	return res, nil
}

func getUploadProtocolInfo(protocolInfos []*gateway.FileUploadProtocol, protocol string) (*gateway.FileUploadProtocol, error) {
	for _, p := range protocolInfos {
		if p.Protocol == protocol {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(protocol)
}

func (action *UploadAction) selectChecksumType(checksumTypes []*provider.ResourceChecksumPriority) provider.ResourceChecksumType {
	var selChecksumType provider.ResourceChecksumType
	var maxPrio uint32 = math.MaxUint32
	for _, xs := range checksumTypes {
		if xs.Priority < maxPrio {
			maxPrio = xs.Priority
			selChecksumType = xs.Type
		}
	}
	return selChecksumType
}

func (action *UploadAction) uploadFilePUT(upload *gateway.FileUploadProtocol, data io.Reader, checksum string, checksumType string) error {
	request, err := action.session.NewHTTPRequest(upload.UploadEndpoint, "PUT", upload.Token, data)
	if err != nil {
		return fmt.Errorf("unable to create HTTP request for '%v': %v", upload.UploadEndpoint, err)
	}

	request.AddParameters(map[string]string{
		"xs":      checksum,
		"xs_type": checksumType,
	})

	_, err = request.Do(true)
	return err
}

func (action *UploadAction) uploadFileTUS(upload *gateway.FileUploadProtocol, target string, data io.Reader, fileInfo os.FileInfo, checksum string, checksumType string) error {
	tusClient, err := net.NewTUSClient(upload.UploadEndpoint, action.session.Token(), upload.Token)
	if err != nil {
		return fmt.Errorf("unable to create TUS client: %v", err)
	}
	return tusClient.Write(data, target, fileInfo, checksumType, checksum)
}

// NewUploadAction creates a new upload action.
func NewUploadAction(session *sdk.Session) (*UploadAction, error) {
	action := &UploadAction{}
	if err := action.initAction(session); err != nil {
		return nil, fmt.Errorf("unable to create the UploadAction: %v", err)
	}
	return action, nil
}

// MustNewUploadAction creates a new upload action and panics on failure.
func MustNewUploadAction(session *sdk.Session) *UploadAction {
	action, err := NewUploadAction(session)
	if err != nil {
		panic(err)
	}
	return action
}
