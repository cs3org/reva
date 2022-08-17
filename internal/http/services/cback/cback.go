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
	"io"
	"net/http"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/go-chi/chi/v5"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

type requestTemp struct {
	BackupID int    `json:"backup_id"`
	Pattern  string `json:"pattern"`
	Snapshot string `json:"snapshot"`
	// destination string
	// enabled     bool
	// date        string
}

func init() {
	global.Register("cback", New)
}

// New returns a new helloworld service
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()
	r := chi.NewRouter()
	s := &svc{
		conf:   conf,
		router: r,
		client: rhttp.GetHTTPClient(),
	}

	if err := s.routerInit(); err != nil {
		return nil, err
	}

	return s, nil

}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

type config struct {
	Prefix            string `mapstructure:"prefix"`
	ImpersonatorToken string `mapstructure:"token"`
	APIURL            string `mapstructure:"api_url"`
}

func (c *config) init() {

	if c.Prefix == "" {
		c.Prefix = "cback"
	}
}

type svc struct {
	conf   *config
	router *chi.Mux
	client *http.Client
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return nil
}

func (s *svc) routerInit() error {

	s.router.Get("/restore", s.handleListJobs)
	s.router.Post("/restore", s.handleRestoreID)
	s.router.Get("/restore/{restore_id}", s.handleRestoreStatus)
	return nil
}

func (s *svc) handleRestoreID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, inContext := ctxpkg.ContextGetUser(ctx)

	if !inContext {
		http.Error(w, errtypes.UserRequired("no user found in context").Error(), http.StatusInternalServerError)
		return
	}

	url := s.conf.APIURL + "/restores/"
	var ssID, searchPath string

	path := r.URL.Query().Get("path")

	if path == "" {
		http.Error(w, "The id query parameter is missing", http.StatusBadRequest)
		return
	}

	resp, err := s.matchBackups(user.Username, path)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resp == nil {
		http.Error(w, errtypes.NotFound("cback: not found").Error(), http.StatusInternalServerError)
		return
	}

	snapshotList, err := s.listSnapshots(user.Username, resp.ID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resp.Substring != "" {
		ssID, searchPath = s.pathTrimmer(snapshotList, resp)

		if ssID == "" {
			http.Error(w, errtypes.NotFound("cback: snapshot not found").Error(), http.StatusNotFound)
			return
		}

		err = s.checkFileType(resp.ID, ssID, user.Username, searchPath, resp.Source)

		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		structbody := &requestTemp{
			BackupID: resp.ID,
			Snapshot: ssID,
			Pattern:  searchPath,
		}

		jbody, err := json.Marshal(structbody)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp, err := s.getRequest(user.Username, url, http.MethodPost, bytes.NewBuffer(jbody))

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		defer resp.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(w).Encode(resp)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		if _, err := io.Copy(w, resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	} else {

		err = errtypes.NotFound("cback: resource not found")
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
}

func (s *svc) handleListJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, inContext := ctxpkg.ContextGetUser(ctx)

	if !inContext {
		http.Error(w, errtypes.UserRequired("no user found in context").Error(), http.StatusInternalServerError)
		return
	}

	url := s.conf.APIURL + "/restores/"

	resp, err := s.getRequest(user.Username, url, http.MethodGet, nil)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer resp.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(resp)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	if _, err := io.Copy(w, resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}

func (s *svc) handleRestoreStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, inContext := ctxpkg.ContextGetUser(ctx)

	if !inContext {
		http.Error(w, errtypes.UserRequired("no user found in context").Error(), http.StatusInternalServerError)
		return
	}

	restoreID := chi.URLParam(r, "restore_id")

	url := s.conf.APIURL + "/restores/" + restoreID
	resp, err := s.getRequest(user.Username, url, http.MethodGet, nil)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer resp.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(resp)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	if _, err := io.Copy(w, resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.router.ServeHTTP(w, r)
	})
}
