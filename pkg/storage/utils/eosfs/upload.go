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

package eosfs

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

func (fs *eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	upload, err := fs.GetUpload(ctx, ref.GetPath())
	if err != nil {
		// Upload corresponding to this ID was not found.
		// Assume that this corresponds to the resource path to which the file has to be uploaded.

		// Set the length to 0 and set SizeIsDeferred to true
		metadata := map[string]string{"sizedeferred": "true"}
		uploadIDs, err := fs.InitiateUpload(ctx, ref, 0, metadata)
		if err != nil {
			return err
		}
		if upload, err = fs.GetUpload(ctx, uploadIDs["simple"]); err != nil {
			return errors.Wrap(err, "ocfs: error retrieving upload")
		}
	}

	return fs.FinishUpload(ctx, upload, r)
}

func (fs *eosfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}
	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": fn,
		},
		ID:   uuid.New().String(),
		Size: uploadLength,
	}

	if metadata != nil {
		if metadata["mtime"] != "" {
			info.MetaData["mtime"] = metadata["mtime"]
		}
		if metadata["chunk-size"] != "" {
			info.MetaData["chunk-size"] = metadata["chunk-size"]
		}
		if metadata["total-length"] != "" {
			info.MetaData["total-length"] = metadata["total-length"]
		}
		if metadata["chunked"] != "" {
			info.MetaData["chunked"] = metadata["chunked"]
		}
		if _, ok := metadata["sizedeferred"]; ok {
			info.SizeIsDeferred = true
		}
	}

	if !info.SizeIsDeferred && info.Size == 0 {
		// no need to create info file and finish directly
		if err = fs.FinishUpload(ctx, info, nil); err != nil {
			return nil, err
		}
	} else {
		// write the info file to disk
		data, err := json.Marshal(info)
		if err != nil {
			return nil, err
		}
		if err = ioutil.WriteFile(fs.getUploadInfoPath(ctx, info.ID), data, defaultFilePerm); err != nil {
			return nil, err
		}
	}

	return map[string]string{
		"simple": info.ID,
	}, nil
}

func (fs *eosfs) getUploadInfoPath(ctx context.Context, uploadID string) string {
	return filepath.Join(fs.conf.CacheDirectory, fs.getLayout(ctx), "uploads", uploadID)
}

func (fs *eosfs) GetUpload(ctx context.Context, id string) (tusd.FileInfo, error) {
	var info tusd.FileInfo
	data, err := ioutil.ReadFile(fs.getUploadInfoPath(ctx, id))
	if err != nil {
		return tusd.FileInfo{}, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return tusd.FileInfo{}, err
	}
	return info, nil
}

func (fs *eosfs) FinishUpload(ctx context.Context, upload tusd.FileInfo, r io.ReadCloser) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}

	url, err := url.Parse(fs.conf.HTTPURL)
	if err != nil {
		return err
	}
	url.Path = path.Join(url.Path, upload.MetaData["filename"])

	req, err := http.NewRequest("PUT", url.String(), r)
	if err != nil {
		return err
	}

	req.Header.Set("Remote-User", u.Id.GetOpaqueId())
	req.Header.Set("Host", host)
	req.Header.Set("X-Real-IP", host)
	req.Header.Set("X-Forwarded-For", host)
	req.Header.Set("CBOX-SKIP-LOCATION-ON-MOVE", "1")
	req.Header.Set("Connection", "")

	if val, ok := upload.MetaData["chunk-size"]; ok {
		req.Header.Set("Oc-Chunk-Size", val)
	}
	if val, ok := upload.MetaData["total-length"]; ok {
		req.Header.Set("Oc-Total-Length", val)
	}
	if val, ok := upload.MetaData["chunked"]; ok {
		req.Header.Set("Oc-Chunked", val)
	}

	// EOS MGM returns a 307 redirect with the endpoint for an FST.
	// The go HTTP client handles redirects with the same parameters as the
	// original request.
	resp, err := fs.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return errors.New("eos: HTTP PUT call returned " + resp.Status)
	}

	return fs.TerminateUpload(ctx, upload)
}

func (fs *eosfs) TerminateUpload(ctx context.Context, upload tusd.FileInfo) error {
	if err := os.Remove(fs.getUploadInfoPath(ctx, upload.ID)); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
