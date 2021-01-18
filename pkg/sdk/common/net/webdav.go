// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this filePath except in compliance with the License.
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

package net

import (
	"fmt"
	"io"
	"io/ioutil"
	"strconv"

	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/studio-b12/gowebdav"

	"github.com/cs3org/reva/pkg/sdk/common"
)

const (
	// WebDAVTokenName is the header name of the WebDAV token.
	WebDAVTokenName = "webdav-token"
	// WebDAVPathName is the header name of the WebDAV file path.
	WebDAVPathName = "webdav-file-path"
)

// WebDAVClient is a simple client wrapper for down- and uploading files via WebDAV.
type WebDAVClient struct {
	client *gowebdav.Client
}

func (webdav *WebDAVClient) initClient(endpoint string, userName string, password string, accessToken string) error {
	// Create the WebDAV client
	webdav.client = gowebdav.NewClient(endpoint, userName, password)

	if accessToken != "" {
		webdav.client.SetHeader(AccessTokenName, accessToken)
	}

	return nil
}

// Read reads all data of the specified remote file.
func (webdav *WebDAVClient) Read(file string) ([]byte, error) {
	reader, err := webdav.client.ReadStream(file)
	if err != nil {
		return nil, fmt.Errorf("unable to create reader: %v", err)
	}
	defer reader.Close()

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("unable to read the data: %v", err)
	}
	return data, nil
}

// Write writes data to the specified remote file.
func (webdav *WebDAVClient) Write(file string, data io.Reader, size int64) error {
	webdav.client.SetHeader("Upload-Length", strconv.FormatInt(size, 10))

	if err := webdav.client.WriteStream(file, data, 0700); err != nil {
		return fmt.Errorf("unable to write the data: %v", err)
	}

	return nil
}

// Remove deletes the entire file/path.
func (webdav *WebDAVClient) Remove(path string) error {
	if err := webdav.client.Remove(path); err != nil {
		return fmt.Errorf("error removing '%v' :%v", path, err)
	}

	return nil
}

func newWebDAVClient(endpoint string, userName string, password string, accessToken string) (*WebDAVClient, error) {
	client := &WebDAVClient{}
	if err := client.initClient(endpoint, userName, password, accessToken); err != nil {
		return nil, fmt.Errorf("unable to create the WebDAV client: %v", err)
	}
	return client, nil
}

// NewWebDAVClientWithAccessToken creates a new WebDAV client using an access token.
func NewWebDAVClientWithAccessToken(endpoint string, accessToken string) (*WebDAVClient, error) {
	return newWebDAVClient(endpoint, "", "", accessToken)
}

// NewWebDAVClientWithOpaque creates a new WebDAV client using the information stored in the opaque.
func NewWebDAVClientWithOpaque(endpoint string, opaque *types.Opaque) (*WebDAVClient, map[string]string, error) {
	values, err := common.GetValuesFromOpaque(opaque, []string{WebDAVTokenName, WebDAVPathName}, true)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid opaque object: %v", err)
	}

	client, err := NewWebDAVClientWithAccessToken(endpoint, values[WebDAVTokenName])
	if err != nil {
		return nil, nil, err
	}
	return client, values, nil
}

// NewWebDAVClient creates a new WebDAV client with user credentials.
func NewWebDAVClient(endpoint string, userName string, password string) (*WebDAVClient, error) {
	return newWebDAVClient(endpoint, userName, password, "")
}
