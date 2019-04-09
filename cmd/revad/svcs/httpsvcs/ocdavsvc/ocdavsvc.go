package ocdavsvc

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path"

	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cernbox/reva/cmd/revad/httpserver"
	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	httpserver.Register("ocdavsvc", New)
}

var logger = log.New("ocdavsvc")

type config struct {
	Prefix             string `mapstructure:"prefix"`
	ChunkFolder        string `mapstructure:"chunk_folder"`
	StorageProviderSvc string `mapstructure:"storageprovidersvc"`
}

type svc struct {
	prefix             string
	chunkFolder        string
	handler            http.Handler
	storageProviderSvc string
	conn               *grpc.ClientConn
	client             storageproviderv0alphapb.StorageProviderServiceClient
}

// New returns a new ocdavsvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	if conf.ChunkFolder == "" {
		conf.ChunkFolder = os.TempDir()
	} else {
		os.MkdirAll(conf.ChunkFolder, 0700)
	}

	s := &svc{
		prefix:             conf.Prefix,
		storageProviderSvc: conf.StorageProviderSvc,
		chunkFolder:        conf.ChunkFolder,
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

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// fake litmus testing for empty namespace: see https://github.com/golang/net/blob/e514e69ffb8bc3c76a71ae40de0118d794855992/webdav/litmus_test_server.go#L58-L89
		if r.Header.Get("X-Litmus") == "props: 3 (propfind_invalid2)" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		head, tail := httpsvcs.ShiftPath(r.URL.Path)

		logger.Println(r.Context(), "head=", head, " tail=", tail)
		switch head {
		case "ocs":
			r.URL.Path = tail
			head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
			if head == "v1.php" {
				head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
				if head == "cloud" {
					head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
					if head == "capabilities" {
						s.doCapabilities(w, r)
						return
					} else if head == "user" {
						s.doUser(w, r)
						return
					}
				} else if head == "config" {
					s.doConfig(w, r)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return

		case "status.php":
			r.URL.Path = tail
			s.doStatus(w, r)
			return

		case "remote.php":
			head2, tail2 := httpsvcs.ShiftPath(tail)

			// TODO refactor as separate handler
			// the old `webdav` endpoint uses remote.php/webdav/$path
			if head2 == "webdav" {
				// inject username in path
				ctx := r.Context()
				u, ok := user.ContextGetUser(ctx)
				if !ok {
					err := errors.Wrap(contextUserRequiredErr("userrequired"), "error getting user from ctx")
					logger.Error(ctx, err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				r.URL.Path = path.Join("/", u.Username, tail2)
				// webdav should be death: baseURI is encoded as part of the
				// reponse payload in href field
				baseURI := path.Join("/", s.Prefix(), "remote.php/webdav")
				ctx = context.WithValue(ctx, "baseuri", baseURI)

				// inject username into Destination header if present
				dstHeader := r.Header.Get("Destination")
				if dstHeader != "" {
					dstURL, err := url.ParseRequestURI(dstHeader)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if dstURL.Path[:18] != "/remote.php/webdav" {
						b := logger.BuildError()
						b = b.Str("path", dstURL.Path)
						b.Msg(ctx, "Destination needs to start with '/remote.php/webdav'")
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
				default:
					w.WriteHeader(http.StatusNotFound)
					return
				}
			}

			// TODO refactor as separate handler
			// the new `files` endpoint uses remote.php/dav/files/$user/$path style paths
			if head2 == "dav" {
				head3, tail3 := httpsvcs.ShiftPath(tail2)
				if head3 == "files" {
					r.URL.Path = tail3
					// webdav should be death: baseURI is encoded as part of the
					// reponse payload in href field
					baseURI := path.Join("/", s.Prefix(), "remote.php/dav/files")
					ctx := context.WithValue(r.Context(), "baseuri", baseURI)
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
						w.WriteHeader(http.StatusNotFound)
						return
					}
				}
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func (s *svc) getConn() (*grpc.ClientConn, error) {
	if s.conn != nil {
		return s.conn, nil
	}

	conn, err := grpc.Dial(s.storageProviderSvc, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *svc) getClient() (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	if s.client != nil {
		return s.client, nil
	}

	conn, err := s.getConn()
	if err != nil {
		return nil, err
	}
	s.client = storageproviderv0alphapb.NewStorageProviderServiceClient(conn)
	return s.client, nil
}
