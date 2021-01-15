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
	"fmt"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storage "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/sdk"
	"github.com/cs3org/reva/pkg/sdk/common/net"
)

// DownloadAction is used to download files through Reva.
// WebDAV will be used automatically if the endpoint supports it.
type DownloadAction struct {
	action
}

// DownloadFile retrieves the data of the provided file path.
// The method first tries to retrieve information about the remote file by performing a "stat" on it.
func (action *DownloadAction) DownloadFile(path string) ([]byte, error) {
	// Get the ResourceInfo object of the specified path
	fileInfoAct := MustNewFileOperationsAction(action.session)
	info, err := fileInfoAct.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("the path '%v' was not found: %v", path, err)
	}

	return action.Download(info)
}

// Download retrieves the data of the provided resource.
func (action *DownloadAction) Download(fileInfo *storage.ResourceInfo) ([]byte, error) {
	if fileInfo.Type != storage.ResourceType_RESOURCE_TYPE_FILE {
		return nil, fmt.Errorf("resource is not a file")
	}

	// Issue a file download request to Reva; this will provide the endpoint to read the file data from
	download, err := action.initiateDownload(fileInfo)
	if err != nil {
		return nil, err
	}

	p, err := getDownloadProtocolInfo(download.Protocols, "simple")
	if err != nil {
		return nil, err
	}

	// Try to get the file via WebDAV first
	if client, values, err := net.NewWebDAVClientWithOpaque(p.DownloadEndpoint, p.Opaque); err == nil {
		data, err := client.Read(values[net.WebDAVPathName])
		if err != nil {
			return nil, fmt.Errorf("error while reading from '%v' via WebDAV: %v", p.DownloadEndpoint, err)
		}
		return data, nil
	}

	// WebDAV is not supported, so directly read the HTTP endpoint
	request, err := action.session.NewHTTPRequest(p.DownloadEndpoint, "GET", p.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create an HTTP request for '%v': %v", p.DownloadEndpoint, err)
	}

	data, err := request.Do(true)
	if err != nil {
		return nil, fmt.Errorf("error while reading from '%v' via HTTP: %v", p.DownloadEndpoint, err)
	}
	return data, nil
}

func (action *DownloadAction) initiateDownload(fileInfo *storage.ResourceInfo) (*gateway.InitiateFileDownloadResponse, error) {
	// Initiating a download request gets us the download endpoint for the specified resource
	req := &provider.InitiateFileDownloadRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: fileInfo.Path,
			},
		},
	}
	res, err := action.session.Client().InitiateFileDownload(action.session.Context(), req)
	if err := net.CheckRPCInvocation("initiating download", res, err); err != nil {
		return nil, err
	}
	return res, nil
}

func getDownloadProtocolInfo(protocolInfos []*gateway.FileDownloadProtocol, protocol string) (*gateway.FileDownloadProtocol, error) {
	for _, p := range protocolInfos {
		if p.Protocol == protocol {
			return p, nil
		}
	}
	return nil, errtypes.NotFound(protocol)
}

// NewDownloadAction creates a new download action.
func NewDownloadAction(session *sdk.Session) (*DownloadAction, error) {
	action := &DownloadAction{}
	if err := action.initAction(session); err != nil {
		return nil, fmt.Errorf("unable to create the DownloadAction: %v", err)
	}
	return action, nil
}

// MustNewDownloadAction creates a new download action and panics on failure.
func MustNewDownloadAction(session *sdk.Session) *DownloadAction {
	action, err := NewDownloadAction(session)
	if err != nil {
		panic(err)
	}
	return action
}
