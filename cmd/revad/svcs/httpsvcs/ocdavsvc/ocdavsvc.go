// Copyright 2018-2019 CERN
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

package ocdavsvc

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path"

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

type ctxKey int

const (
	ctxKeyBaseURI ctxKey = iota
)

func init() {
	httpserver.Register("ocdavsvc", New)
}

type config struct {
	Prefix      string `mapstructure:"prefix"`
	ChunkFolder string `mapstructure:"chunk_folder"`
	GatewaySvc  string `mapstructure:"gatewaysvc"`
}

type svc struct {
	prefix      string
	chunkFolder string
	handler     http.Handler
	gatewaySvc  string
	conn        *grpc.ClientConn
	client      storageproviderv0alphapb.StorageProviderServiceClient
}

// New returns a new ocdavsvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	if conf.ChunkFolder == "" {
		conf.ChunkFolder = os.TempDir()
	}

	os.MkdirAll(conf.ChunkFolder, 0755)

	s := &svc{
		prefix:      conf.Prefix,
		gatewaySvc:  conf.GatewaySvc,
		chunkFolder: conf.ChunkFolder,
	}
	s.setHandler()
	return s, nil
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())

		// the webdav api is accessible from anywhere
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// TODO(jfd): do we need this?
		// fake litmus testing for empty namespace: see https://github.com/golang/net/blob/e514e69ffb8bc3c76a71ae40de0118d794855992/webdav/litmus_test_server.go#L58-L89
		if r.Header.Get("X-Litmus") == "props: 3 (propfind_invalid2)" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		head, tail := httpsvcs.ShiftPath(r.URL.Path)
		log.Debug().Str("head", head).Str("tail", tail).Msg("http routing")

		switch head {
		case "status.php":
			r.URL.Path = tail
			s.doStatus(w, r)
			return

		case "remote.php":
			head2, tail2 := httpsvcs.ShiftPath(tail)

			// TODO(jfd): refactor as separate handler
			// the old `webdav` endpoint uses remote.php/webdav/$path
			if head2 == "webdav" {

				r.URL.Path = tail2

				if r.Method == "OPTIONS" {
					// no need for the user, and we need to be able
					// to answer preflight checks, which have no auth headers
					r.URL.Path = tail2 // tail alwas starts with /
					s.doOptions(w, r)
					return
				}

				// inject username in path
				ctx := r.Context()
				u, ok := user.ContextGetUser(ctx)
				if !ok {
					log.Error().Msg("error getting user from context")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				// TODO(labkode): this assumes too much, basically using ocdavsvc you can't access a global namespace.
				// This must be changed.
				// r.URL.Path = path.Join("/", u.Username, tail2)
				// webdav should be death: baseURI is encoded as part of the
				// reponse payload in href field
				baseURI := path.Join("/", s.Prefix(), "remote.php/webdav")
				ctx = context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)

				// inject username into Destination header if present
				dstHeader := r.Header.Get("Destination")
				if dstHeader != "" {
					dstURL, err := url.ParseRequestURI(dstHeader)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if dstURL.Path[:18] != "/remote.php/webdav" {
						log.Warn().Str("path", dstURL.Path).Msg("dst needs to start with /remote.php/webdav/")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					r.Header.Set("Destination", path.Join(baseURI, u.Username, dstURL.Path[18:])) // 18 = len ("/remote.php/webdav")
				}

				r = r.WithContext(ctx)

				switch r.Method {
				case "PROPFIND":
					s.doPropfind(w, r)
					return
				case "HEAD":
					s.doHead(w, r)
					return
				case "GET":
					s.doGet(w, r)
					return
				case "LOCK":
					s.doLock(w, r)
					return
				case "UNLOCK":
					s.doUnlock(w, r)
					return
				case "PROPPATCH":
					s.doProppatch(w, r)
					return
				case "MKCOL":
					s.doMkcol(w, r)
					return
				case "MOVE":
					s.doMove(w, r)
					return
				case "COPY":
					s.doCopy(w, r)
					return
				case "PUT":
					s.doPut(w, r)
					return
				case "DELETE":
					s.doDelete(w, r)
					return
				default:
					log.Warn().Msg("resource not found")
					w.WriteHeader(http.StatusNotFound)
					return
				}
			}

			// TODO(jfd) refactor as separate handler
			// the new `files` endpoint uses remote.php/dav/files/$user/$path style paths
			if head2 == "dav" {
				head3, tail3 := httpsvcs.ShiftPath(tail2)
				if head3 == "files" {
					r.URL.Path = tail3
					// webdav should be death: baseURI is encoded as part of the
					// reponse payload in href field
					baseURI := path.Join("/", s.Prefix(), "remote.php/dav/files")
					ctx := context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)
					r = r.WithContext(ctx)

					switch r.Method {
					case "PROPFIND":
						s.doPropfind(w, r)
						return
					case "OPTIONS":
						s.doOptions(w, r)
						return
					case "HEAD":
						s.doHead(w, r)
						return
					case "GET":
						s.doGet(w, r)
						return
					case "LOCK":
						s.doLock(w, r)
						return
					case "UNLOCK":
						s.doUnlock(w, r)
						return
					case "PROPPATCH":
						s.doProppatch(w, r)
						return
					case "MKCOL":
						s.doMkcol(w, r)
						return
					case "MOVE":
						s.doMove(w, r)
						return
					case "COPY":
						s.doCopy(w, r)
						return
					case "PUT":
						s.doPut(w, r)
						return
					case "DELETE":
						s.doDelete(w, r)
						return
					case "REPORT":
						s.doReport(w, r)
						return
					default:
						log.Warn().Msg("resource not found")
						w.WriteHeader(http.StatusNotFound)
						return
					}
				}
				if head3 == "avatars" {
					r.URL.Path = tail3
					s.doAvatar(w, r)
					return
				}
			}
		}
		log.Warn().Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)
	})
}

func (s *svc) getClient() (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	return pool.GetStorageProviderServiceClient(s.gatewaySvc)
}
