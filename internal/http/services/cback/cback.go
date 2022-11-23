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

package cback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"text/template"

	"github.com/Masterminds/sprig"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storage "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	cbackfs "github.com/cs3org/reva/pkg/storage/fs/cback"
	"github.com/cs3org/reva/pkg/storage/utils/cback"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/go-chi/chi/v5"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("cback", New)
}

const webdavPrefix = "/remote.php/dav/files/"

type config struct {
	Prefix            string `mapstructure:"prefix"`
	Token             string `mapstructure:"token"`
	URL               string `mapstructure:"url"`
	Insecure          bool   `mapstructure:"insecure"`
	Timeout           int    `mapstructure:"timeout"`
	GatewaySvc        string `mapstructure:"gatewaysvc"`
	StorageID         string `mapstructure:"storage_id"`
	StorageMount      string `mapstructure:"storage_mount"`
	TemplateToStorage string `mapstructure:"template_to_storage"`
}

type svc struct {
	config *config
	router *chi.Mux
	client *cback.Client
	gw     gateway.GatewayAPIClient
	tpl    *template.Template
}

// New returns a new cback http service
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, errors.Wrap(err, "cback: error decodinf config")
	}

	c.init()

	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "cback: error getting gateway client")
	}

	tplStorage, err := template.New("tpl_storage").Funcs(sprig.TxtFuncMap()).Parse(c.TemplateToStorage)
	if err != nil {
		return nil, errors.Wrap(err, "cback: error creating template")
	}

	r := chi.NewRouter()
	s := &svc{
		config: c,
		gw:     gw,
		router: r,
		client: cback.New(&cback.Config{
			URL:      c.URL,
			Token:    c.Token,
			Insecure: c.Insecure,
			Timeout:  c.Timeout,
		}),
		tpl: tplStorage,
	}

	s.initRouter()

	return s, nil

}

// Close cleanup the cback http service
func (s *svc) Close() error {
	return nil
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "cback"
	}
	if c.StorageMount == "" {
		c.StorageMount = "/cback"
	}
	if c.TemplateToStorage == "" {
		c.TemplateToStorage = "{{.}}"
	}
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

func (s *svc) Prefix() string {
	return s.config.Prefix
}

func (s *svc) Unprotected() []string {
	return nil
}

func (s *svc) initRouter() {
	s.router.Get("/restores", s.getRestores)
	s.router.Get("/restores/{id}", s.getRestoreByID)
	s.router.Post("/restores", s.createRestore)

	s.router.Get("/backups", s.getBackups)
}

type destination struct {
	Path   string `json:"path"`
	Webdav string `json:"webdav"`
}

type restoreOut struct {
	ID          int         `json:"id"`
	Path        string      `json:"path"`
	Destination destination `json:"destination"`
	Status      int         `json:"status"`
}

func (s *svc) convertToRestoureOut(user *userpb.User, r *cback.Restore) *restoreOut {
	return &restoreOut{
		ID:          r.ID,
		Path:        r.Pattern,
		Destination: utils.Must(s.toDestination(user.Username, r.Destionation)),
		Status:      r.Status,
	}
}

func (s *svc) createRestore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	stat, err := s.gw.Stat(ctx, &storage.StatRequest{
		Ref: &storage.Reference{
			Path: path,
		},
	})

	switch {
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	case stat.Status.Code == rpc.Code_CODE_NOT_FOUND:
		http.Error(w, stat.Status.Message, http.StatusNotFound)
		return
	case stat.Status.Code != rpc.Code_CODE_OK:
		http.Error(w, stat.Status.Message, http.StatusInternalServerError)
		return
	}

	if stat.Info.Id == nil || stat.Info.Id.StorageId != s.config.StorageID {
		http.Error(w, fmt.Sprintf("path not belonging to %s storage driver", s.config.StorageID), http.StatusBadRequest)
		return
	}

	path, snapshotID, backupID, ok := cbackfs.GetBackupInfo(stat.Info.Id)
	if !ok {
		http.Error(w, "cannot restore the given path", http.StatusBadRequest)
		return
	}

	restore, err := s.client.NewRestore(ctx, user.Username, backupID, path, snapshotID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, s.convertToRestoureOut(user, restore))
}

func (s *svc) getRestores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	list, err := s.client.ListRestores(ctx, user.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := make([]*restoreOut, 0, len(list))
	for _, r := range list {
		res = append(res, s.convertToRestoureOut(user, r))
	}

	s.writeJSON(w, res)
}

func (s *svc) writeJSON(w http.ResponseWriter, r any) {
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(r)
}

func (s *svc) getRestoreByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	restoreID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	restore, err := s.client.GetRestore(ctx, user.Username, int(restoreID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, s.convertToRestoureOut(user, restore))
}

func getPath(p string, tpl *template.Template) (string, error) {
	var b bytes.Buffer
	if err := tpl.Execute(&b, p); err != nil {
		return "", err
	}
	return b.String(), nil
}

func (s *svc) getBackups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	list, err := s.client.ListBackups(ctx, user.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	paths := make([]destination, 0, len(list))
	for _, b := range list {
		d, err := s.toDestination(user.Username, b.Source)
		if err != nil {
			continue
		}
		paths = append(paths, d)
	}

	s.writeJSON(w, paths)
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.router.ServeHTTP(w, r)
	})
}

func (s *svc) toDestination(username, p string) (destination, error) {
	p, err := getPath(p, s.tpl)
	if err != nil {
		return destination{}, err
	}
	p = path.Join(s.config.StorageMount, p)
	return destination{
		Path:   p,
		Webdav: getWebdavPath(username, p),
	}, nil
}

func getWebdavPath(username, p string) string {
	return path.Join(webdavPrefix, username, p)
}
