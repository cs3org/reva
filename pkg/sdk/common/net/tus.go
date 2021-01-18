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
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/eventials/go-tus"
	"github.com/eventials/go-tus/memorystore"

	"github.com/cs3org/reva/pkg/sdk/common"
)

// TUSClient is a simple client wrapper for uploading files via TUS.
type TUSClient struct {
	config *tus.Config
	client *tus.Client

	supportsResourceCreation bool
}

func (client *TUSClient) initClient(endpoint string, accessToken string, transportToken string) error {
	// Create the TUS configuration
	client.config = tus.DefaultConfig()
	client.config.Resume = true

	memStore, err := memorystore.NewMemoryStore()
	if err != nil {
		return fmt.Errorf("unable to create a TUS memory store: %v", err)
	}
	client.config.Store = memStore

	client.config.Header.Add(AccessTokenName, accessToken)
	client.config.Header.Add(TransportTokenName, transportToken)

	// Create the TUS client
	tusClient, err := tus.NewClient(endpoint, client.config)
	if err != nil {
		return fmt.Errorf("error creating the TUS client: %v", err)
	}
	client.client = tusClient

	// Check if the TUS server supports resource creation
	client.supportsResourceCreation = client.checkEndpointCreationOption(endpoint)

	return nil
}

func (client *TUSClient) checkEndpointCreationOption(endpoint string) bool {
	// Perform an OPTIONS request to the endpoint; if this succeeds, check if the header "Tus-Extension" contains the "creation" flag
	httpClient := &http.Client{
		Timeout: time.Duration(1.5 * float64(time.Second)),
	}

	if httpReq, err := http.NewRequest("OPTIONS", endpoint, nil); err == nil {
		if res, err := httpClient.Do(httpReq); err == nil {
			defer res.Body.Close()

			if res.StatusCode == http.StatusOK {
				ext := strings.Split(res.Header.Get("Tus-Extension"), ",")
				return common.FindStringNoCase(ext, "creation") != -1
			}
		}
	}

	return false
}

// Write writes the provided data to the endpoint.
// The target is used as the filename on the remote site. The file information and checksum are used to create a fingerprint.
func (client *TUSClient) Write(data io.Reader, target string, fileInfo os.FileInfo, checksumType string, checksum string) error {
	metadata := map[string]string{
		"filename": path.Base(target),
		"dir":      path.Dir(target),
		"checksum": fmt.Sprintf("%s %s", checksumType, checksum),
	}
	fingerprint := fmt.Sprintf("%s-%d-%s-%s", path.Base(target), fileInfo.Size(), fileInfo.ModTime(), checksum)

	upload := tus.NewUpload(data, fileInfo.Size(), metadata, fingerprint)
	client.config.Store.Set(upload.Fingerprint, client.client.Url)

	var uploader *tus.Uploader
	if client.supportsResourceCreation {
		upldr, err := client.client.CreateUpload(upload)
		if err != nil {
			return fmt.Errorf("unable to perform the TUS resource creation for '%v': %v", client.client.Url, err)
		}
		uploader = upldr
	} else {
		uploader = tus.NewUploader(client.client, client.client.Url, upload, 0)
	}

	if err := uploader.Upload(); err != nil {
		return fmt.Errorf("unable to perform the TUS upload for '%v': %v", client.client.Url, err)
	}

	return nil
}

// NewTUSClient creates a new TUS client.
func NewTUSClient(endpoint string, accessToken string, transportToken string) (*TUSClient, error) {
	client := &TUSClient{}
	if err := client.initClient(endpoint, accessToken, transportToken); err != nil {
		return nil, fmt.Errorf("unable to create the TUS client: %v", err)
	}
	return client, nil
}
