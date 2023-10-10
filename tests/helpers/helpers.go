// Copyright 2018-2023 CERN
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

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/httpclient"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/studio-b12/gowebdav"
)

// TempDir creates a temporary directory in tmp/ and returns its path
//
// Temporary test directories are created in reva/tmp because system
// /tmp directories are often tmpfs mounts which do not support user
// extended attributes.
func TempDir(name string) (string, error) {
	_, currentFileName, _, ok := runtime.Caller(0)
	if !ok {
		return "nil", errors.New("failed to retrieve currentFileName")
	}
	tmpDir := filepath.Join(filepath.Dir(currentFileName), "../../tmp")
	err := os.MkdirAll(tmpDir, 0755)
	if err != nil {
		return "nil", err
	}
	tmpRoot, err := os.MkdirTemp(tmpDir, "reva-tests-*-root")
	if err != nil {
		return "nil", err
	}

	return tmpRoot, nil
}

// TempFile creates a temporary file returning its path.
// The file is filled with the provider r if not nil.
func TempFile(r io.Reader) (string, error) {
	dir, err := TempDir("")
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "*")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if r != nil {
		if _, err := io.Copy(f, r); err != nil {
			return "", err
		}
	}
	return f.Name(), nil
}

// TempJSONFile creates a temporary file returning its path.
// The file is filled with the object encoded in json.
func TempJSONFile(c any) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return TempFile(bytes.NewBuffer(data))
}

// Upload can be used to initiate an upload and do the upload to a storage.FS in one step.
func Upload(ctx context.Context, fs storage.FS, ref *provider.Reference, content []byte) error {
	uploadIds, err := fs.InitiateUpload(ctx, ref, 0, map[string]string{})
	if err != nil {
		return err
	}
	uploadID, ok := uploadIds["simple"]
	if !ok {
		return errors.New("simple upload method not available")
	}
	uploadRef := &provider.Reference{Path: "/" + uploadID}
	err = fs.Upload(ctx, uploadRef, io.NopCloser(bytes.NewReader(content)))
	return err
}

// UploadGateway uploads in one step a the content in a file.
func UploadGateway(ctx context.Context, gw gatewayv1beta1.GatewayAPIClient, ref *provider.Reference, content []byte) error {
	res, err := gw.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Ref: ref,
	})
	if err != nil {
		return errors.Wrap(err, "error initiating file upload")
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return errors.Errorf("error initiating file upload: %s", res.Status.Message)
	}

	var token, endpoint string
	for _, p := range res.Protocols {
		if p.Protocol == "simple" {
			token, endpoint = p.Token, p.UploadEndpoint
		}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(content))
	if err != nil {
		return errors.Wrap(err, "error creating new request")
	}

	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	httpRes, err := httpclient.New().Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "error doing put request")
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		return errors.Errorf("error doing put request: %s", httpRes.Status)
	}

	return nil
}

// Download downloads the content of a file in one step.
func Download(ctx context.Context, gw gatewayv1beta1.GatewayAPIClient, ref *provider.Reference) ([]byte, error) {
	res, err := gw.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{
		Ref: ref,
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}

	var token, endpoint string
	for _, p := range res.Protocols {
		if p.Protocol == "simple" {
			token, endpoint = p.Token, p.DownloadEndpoint
		}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	httpRes, err := httpclient.New().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		return nil, errors.New(httpRes.Status)
	}

	return io.ReadAll(httpRes.Body)
}

// Resource represents a general resource (file or folder).
type Resource interface {
	isResource()
}

// Folder implements the Resource interface.
type Folder map[string]Resource

func (Folder) isResource() {}

// File implements the Resource interface.
type File struct {
	Content string
}

func (File) isResource() {}

// CreateStructure creates the given structure.
func CreateStructure(ctx context.Context, gw gatewayv1beta1.GatewayAPIClient, root string, f Resource) error {
	switch r := f.(type) {
	case Folder:
		if err := CreateFolder(ctx, gw, root); err != nil {
			return err
		}
		for name, resource := range r {
			p := filepath.Join(root, name)
			if err := CreateStructure(ctx, gw, p, resource); err != nil {
				return err
			}
		}
	case File:
		if err := CreateFile(ctx, gw, root, []byte(r.Content)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("resource %T not valid", f)
	}
	return nil
}

// CreateFile creates a file in the given path with an initial content.
func CreateFile(ctx context.Context, gw gatewayv1beta1.GatewayAPIClient, path string, content []byte) error {
	initRes, err := gw.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{Ref: &provider.Reference{Path: path}})
	if err != nil {
		return err
	}
	var token, endpoint string
	for _, p := range initRes.Protocols {
		if p.Protocol == "simple" {
			token, endpoint = p.Token, p.UploadEndpoint
		}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(content))
	if err != nil {
		return err
	}

	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	httpRes, err := httpclient.New().Do(httpReq)
	if err != nil {
		return err
	}
	if httpRes.StatusCode != http.StatusOK {
		return errors.New(httpRes.Status)
	}
	defer httpRes.Body.Close()
	return nil
}

// CreateFolder creates a folder in the given path.
func CreateFolder(ctx context.Context, gw gatewayv1beta1.GatewayAPIClient, path string) error {
	res, err := gw.CreateContainer(ctx, &provider.CreateContainerRequest{
		Ref: &provider.Reference{Path: path},
	})
	if err != nil {
		return err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return errors.New(res.Status.Message)
	}
	return nil
}

// SameContentWebDAV checks that starting from the root path the webdav client sees the same
// content defined in the Resource.
func SameContentWebDAV(cl *gowebdav.Client, root string, f Resource) (bool, error) {
	return sameContentWebDAV(cl, root, "", f)
}

func sameContentWebDAV(cl *gowebdav.Client, root, rel string, f Resource) (bool, error) {
	switch r := f.(type) {
	case Folder:
		list, err := cl.ReadDir(rel)
		if err != nil {
			return false, err
		}
		if len(list) != len(r) {
			return false, nil
		}
		for _, d := range list {
			resource, ok := r[d.Name()]
			if !ok {
				return false, nil
			}
			ok, err := sameContentWebDAV(cl, root, filepath.Join(rel, d.Name()), resource)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	case File:
		c, err := cl.Read(rel)
		if err != nil {
			return false, err
		}
		if !bytes.Equal(c, []byte(r.Content)) {
			return false, nil
		}
		return true, nil
	default:
		return false, errors.New("resource not valid")
	}
}
