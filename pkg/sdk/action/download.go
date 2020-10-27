/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package action

import (
	"fmt"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storage "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

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

	// Try to get the file via WebDAV first
	if client, values, err := net.NewWebDAVClientWithOpaque(download.DownloadEndpoint, download.Opaque); err == nil {
		data, err := client.Read(values[net.WebDAVPathName])
		if err != nil {
			return nil, fmt.Errorf("error while reading from '%v' via WebDAV: %v", download.DownloadEndpoint, err)
		}
		return data, nil
	} else {
		// WebDAV is not supported, so directly read the HTTP endpoint
		request, err := action.session.NewHTTPRequest(download.DownloadEndpoint, "GET", download.Token, nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create an HTTP request for '%v': %v", download.DownloadEndpoint, err)
		}

		data, err := request.Do(true)
		if err != nil {
			return nil, fmt.Errorf("error while reading from '%v' via HTTP: %v", download.DownloadEndpoint, err)
		}
		return data, nil
	}
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
