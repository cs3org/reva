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

package tus

import (
	"net/http"
	"path/filepath"

	"github.com/pkg/errors"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/rhttp/datatx"
	"github.com/cs3org/reva/v2/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/v2/pkg/rhttp/datatx/utils/download"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/mitchellh/mapstructure"
	tusd "github.com/tus/tusd/pkg/handler"
)

func init() {
	registry.Register("tus", New)
}

type config struct{}

type manager struct {
	conf      *config
	publisher events.Publisher
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a datatx manager implementation that relies on HTTP PUT/GET.
func New(m map[string]interface{}, publisher events.Publisher) (datatx.DataTX, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{
		conf:      c,
		publisher: publisher,
	}, nil
}

func (m *manager) Handler(fs storage.FS) (http.Handler, error) {
	composable, ok := fs.(composable)
	if !ok {
		return nil, errtypes.NotSupported("file system does not support the tus protocol")
	}

	sublog := appctx.GetLogger(ctx).With().Str("datatx", "spaces").Str("space", spaceID).Logger()

	// A storage backend for tusd may consist of multiple different parts which
	// handle upload creation, locking, termination and so on. The composer is a
	// place where all those separated pieces are joined together. In this example
	// we only use the file store but you may plug in multiple.
	composer := tusd.NewStoreComposer()

	// let the composable storage tell tus which extensions it supports
	composable.UseIn(composer)

	config := tusd.Config{
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
	}

	handler, err := tusd.NewUnroutedHandler(config)
	if err != nil {
		return nil, err
	}

	if m.publisher != nil {
		go func() {
			for {
				ev := <-handler.CompleteUploads

				u := ev.Upload
				owner := &userv1beta1.UserId{
					Idp:      u.Storage["Idp"],
					OpaqueId: u.Storage["UserId"],
				}
				uploadedEv := events.FileUploaded{
					Owner:     owner,
					Executant: owner,
					Ref: &providerv1beta1.Reference{
						ResourceId: &providerv1beta1.ResourceId{
							StorageId: storagespace.FormatStorageID(u.MetaData["providerID"], u.Storage["SpaceRoot"]),
							OpaqueId:  u.Storage["SpaceRoot"],
						},
						Path: utils.MakeRelativePath(filepath.Join(u.MetaData["dir"], u.MetaData["filename"])),
					},
				}

				if err := events.Publish(m.publisher, uploadedEv); err != nil {
					sublog.Error().Err(err).Msg("failed to publish FileUploaded event")
				}
			}
		}()
	}

	h := handler.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

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
		case "GET":
			download.GetOrHeadFile(w, r, fs, "")
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))

	return h, nil
}

// Composable is the interface that a struct needs to implement
// to be composable, so that it can support the TUS methods
type composable interface {
	UseIn(composer *tusd.StoreComposer)
}
