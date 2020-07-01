// Copyright 2018-2020 CERN
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

package tus

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/datatx"
	"github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
	tusd "github.com/tus/tusd/pkg/handler"
)

func init() {
	registry.Register("tus", New)
}

type manager struct {
	proxy *proxy
}

type config struct{}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a datatx manager implementation that relies on HTTP PUT/GET.
func New(m map[string]interface{}) (datatx.DataTX, error) {
	_, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{}, nil
}

func (m *manager) Handler(prefix string, fs storage.FS) (http.Handler, error) {
	proxy := newProxy(fs)
	composer := tusd.NewStoreComposer()
	proxy.UseIn(composer)

	config := tusd.Config{
		BasePath:      prefix,
		StoreComposer: composer,
	}

	handler, err := tusd.NewUnroutedHandler(config)
	if err != nil {
		return nil, err
	}

	h := handler.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Info().Msgf("tusd routing: path=%s", r.URL.Path)

		method := r.Method
		// https://github.com/tus/tus-resumable-upload-protocol/blob/master/protocol.md#x-http-method-override
		if r.Header.Get("X-HTTP-Method-Override") != "" {
			method = r.Header.Get("X-HTTP-Method-Override")
		}

		switch method {
		case "POST":
			handler.PostFile(w, r)
		case "HEAD":
			handler.HeadFile(w, r)
		case "PATCH":
			handler.PatchFile(w, r)
		case "DELETE":
			handler.DelFile(w, r)
		// TODO(pvince81): allow for range-based requests?
		case "GET":
			ctx := r.Context()
			log := appctx.GetLogger(ctx)
			var fn string
			files, ok := r.URL.Query()["filename"]
			if !ok || len(files[0]) < 1 {
				fn = r.URL.Path
			} else {
				fn = files[0]
			}

			fsfn := strings.TrimPrefix(fn, prefix)
			ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fsfn}}

			rc, err := fs.Download(ctx, ref)
			if err != nil {
				log.Err(err).Msg("datasvc: error downloading file")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			_, err = io.Copy(w, rc)
			if err != nil {
				log.Error().Err(err).Msg("error copying data to response")
				return
			}
			return
		}
	}))

	return h, nil
}

// Composable is the interface that a struct needs to implement to be composable by this composer
type Composable interface {
	UseIn(composer *tusd.StoreComposer)
}

type proxy struct {
	storage.FS
}

func newProxy(fs storage.FS) *proxy {
	return &proxy{FS: fs}
}

func (p *proxy) UseIn(c *tusd.StoreComposer) {
	c.UseCore(p)
	//c.UseTerminater(fs)
	//c.UseConcater(fs)
	//c.UseLengthDeferrer(fs)
}

func (p *proxy) NewUpload(ctx context.Context, info tusd.FileInfo) (upload tusd.Upload, err error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("info", info).Msg("tus: NewUpload")

	fn := info.MetaData["filename"]
	if fn == "" {
		return nil, errors.New("tus: missing filename in metadata")
	}
	info.MetaData["filename"] = path.Clean(info.MetaData["filename"])

	dir := info.MetaData["dir"]
	if dir == "" {
		return nil, errors.New("tus: missing dir in metadata")
	}
	info.MetaData["dir"] = path.Clean(info.MetaData["dir"])

	fullpath := path.Join("/", dir, fn)
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: fullpath,
		},
	}

	id, err := p.InitiateUpload(ctx, ref, info.Size, info.MetaData)
	if err != nil {
		return nil, errors.New("tus: error obtaining upload if from fs")
	}
	info.ID = id
	log.Debug().Interface("", info).Msg("tus: obtained id from fs")

	return nil, nil
}

func (p *proxy) GetUpload(ctx context.Context, id string) (upload tusd.Upload, err error) {
	return nil, nil
}

type fileUpload struct {
	info     tusd.FileInfo
	file     string
	fileInfo string
	fs       storage.FS
}

// GetInfo returns the FileInfo
func (upload *fileUpload) GetInfo(ctx context.Context) (tusd.FileInfo, error) {
	return upload.info, nil
}

// GetReader returns an io.Reader for the upload
func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	ref := &provider.Reference{Spec: &provider.Reference_Path{Path: upload.file}}
	return upload.fs.Download(ctx, ref)
}

// WriteChunk writes the stream from the reader to the given offset of the upload
// TODO use the grpc api to directly stream to a temporary uploads location in the eos shadow tree
func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for OwnCloudStore it's not important whether the stream has ended
	// on purpose or accidentally.
	if err != nil {
		if err != io.ErrUnexpectedEOF {
			return n, err
		}
	}

	upload.info.Offset += n
	err = upload.writeInfo()

	return n, err
}

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *fileUpload) writeInfo() error {
	data, err := json.Marshal(upload.info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// FinishUpload finishes an upload and moves the file to the internal destination
func (upload *fileUpload) FinishUpload(ctx context.Context) error {

	checksum := upload.info.MetaData["checksum"]
	if checksum != "" {
		// check checksum
		s := strings.SplitN(checksum, " ", 2)
		if len(s) == 2 {
			alg, hash := s[0], s[1]

			log := appctx.GetLogger(ctx)
			log.Debug().
				Interface("info", upload.info).
				Str("alg", alg).
				Str("hash", hash).
				Msg("eos: TODO check checksum") // TODO this is done by eos if we write chunks to it directly

		}
	}
	np := filepath.Join(upload.info.MetaData["dir"], upload.info.MetaData["filename"])

	// TODO check etag with If-Match header
	// if destination exists
	//if _, err := os.Stat(np); err == nil {
	// copy attributes of existing file to tmp file befor overwriting the target?
	// eos creates revisions internally
	//}

	err := upload.fs.c.WriteFile(ctx, upload.info.Storage["Username"], np, upload.binPath)

	// only delete the upload if it was successfully written to eos
	if err == nil {
		// cleanup in the background, delete might take a while and we don't need to wait for it to finish
		go func() {
			if err := os.Remove(upload.infoPath); err != nil {
				if !os.IsNotExist(err) {
					log := appctx.GetLogger(ctx)
					log.Err(err).Interface("info", upload.info).Msg("eos: could not delete upload info")
				}
			}
			if err := os.Remove(upload.binPath); err != nil {
				if !os.IsNotExist(err) {
					log := appctx.GetLogger(ctx)
					log.Err(err).Interface("info", upload.info).Msg("eos: could not delete upload binary")
				}
			}
		}()
	}

	// TODO: set mtime if specified in metadata

	// metadata propagation is left to the storage implementation
	return err
}
