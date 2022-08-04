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
	"context"
	"io"
	"net/http"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/go-chi/chi/v5"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

type Request struct {
	backup_id   int
	pattern     string
	snapshot    string
	destination string
	enabled     bool
	date        string
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
	ImpersonatorToken string `mapstructure:"impersonator"`
}

func (c *config) init() {

	if c.Prefix == "" {
		c.Prefix = "cback"
	}
}

type svc struct {
	conf   *config
	router *chi.Mux
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return nil
}

func (s *svc) routerInit() error {

	s.router.Get("/cback/restore", s.handleListJobs)
	s.router.Post("/cback/restore", s.handleRestoreID)
	s.router.Get("/cback/restore/{restore_id}", s.handleRestoreStatus)
	return nil
}

func PostCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "restore_id", chi.URLParam(r, "restore_id"))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *svc) handleRestoreID(w http.ResponseWriter, r *http.Request) {
	/*ctx := r.Context()
	user, _ := ctxpkg.ContextGetUser(ctx)

	theTime := time.Now()

	r.SetBasicAuth(user.Username, s.conf.ImpersonatorToken)

	path := r.URL.Query().Get("path")

	u, err := json.Marshal(Request{backup_id: 0, pattern: path, snapshot: "0", destination: "", enabled: true, date: theTime.Format(time.RFC3339)})

	resp, err := http.Post("http://cback-portal-dev-01:8000/restores")*/

}

func (s *svc) handleListJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := ctxpkg.ContextGetUser(ctx)

	r.SetBasicAuth(user.Username, s.conf.ImpersonatorToken)
	resp, err := http.Get("http://cback-portal-dev-01:8000/restores")

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")

	if _, err := io.Copy(w, resp.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *svc) handleRestoreStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := ctxpkg.ContextGetUser(ctx)

	r.SetBasicAuth(user.Username, s.conf.ImpersonatorToken)

	restoreID := chi.URLParam(r, "restore_id")

	resp, err := http.Get("http://cback-portal-dev-01:8000/restores/" + restoreID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")

	if _, err := io.Copy(w, resp.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.router.ServeHTTP(w, r)

	})
}
