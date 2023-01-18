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
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage"
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

// TempJsonFile creates a temporary file returning its path.
// The file is filled with the object encoded in json.
func TempJsonFile(c any) (string, error) {
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
