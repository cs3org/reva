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

package dataprovider

import (
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
	tusd "github.com/tus/tusd/pkg/handler"
)

func init() {
	global.Register("dataprovider", New)
}

type config struct {
	Prefix   string                            `mapstructure:"prefix" docs:"data;The prefix to be used for this HTTP service"`
	Driver   string                            `mapstructure:"driver" docs:"localhome;The storage driver to be used."`
	Drivers  map[string]map[string]interface{} `mapstructure:"drivers" docs:"url:pkg/storage/fs/localhome/localhome.go;The configuration for the storage driver"`
	Timeout  int64                             `mapstructure:"timeout"`
	Insecure bool                              `mapstructure:"insecure"`
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "data"
	}
	if c.Driver == "" {
		c.Driver = "localhome"
	}
}

type svc struct {
	conf    *config
	handler http.Handler
	storage storage.FS
}

// New returns a new datasvc
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()

	fs, err := getFS(conf)
	if err != nil {
		return nil, err
	}

	s := &svc{
		storage: fs,
		conf:    conf,
	}

	err = s.setHandler()
	return s, err
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return []string{}
}

// Create a new DataStore instance which is responsible for
// storing the uploaded file on disk in the specified directory.
// This path _must_ exist before we store uploads in it.
func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() error {

	tusHandler := s.getTusHandler()

	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Info().Msgf("dataprovider routing: path=%s", r.URL.Path)

		method := r.Method
		// https://github.com/tus/tus-resumable-upload-protocol/blob/master/protocol.md#x-http-method-override
		if r.Header.Get("X-HTTP-Method-Override") != "" {
			method = r.Header.Get("X-HTTP-Method-Override")
		}

		switch method {
		// old fashioned download.
		// GET is not part of the tus.io protocol
		// TODO allow range based get requests? that end before the current offset
		case "GET":
			s.doGet(w, r)
		case "PUT":
			s.doPut(w, r)
		case "HEAD":
			w.WriteHeader(http.StatusOK)

		// tus.io based uploads
		// uploads are initiated using the CS3 APIs Initiate Upload call
		case "POST":
			if tusHandler != nil {
				tusHandler.PostFile(w, r)
			} else {
				w.WriteHeader(http.StatusNotImplemented)
			}
		case "PATCH":
			if tusHandler != nil {
				tusHandler.PatchFile(w, r)
			} else {
				w.WriteHeader(http.StatusNotImplemented)
			}
		// TODO Only attach the DELETE handler if the Terminate() method is provided
		case "DELETE":
			if tusHandler != nil {
				tusHandler.DelFile(w, r)
			} else {
				w.WriteHeader(http.StatusNotImplemented)
			}
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	})

	return nil
}

// Composable is the interface that a struct needs to implement
// to be composable, so that it can support the TUS methods
type composable interface {
	UseIn(composer *tusd.StoreComposer)
}

func (s *svc) getTusHandler() *tusd.UnroutedHandler {
	composable, ok := s.storage.(composable)
	if ok {
		// A storage backend for tusd may consist of multiple different parts which
		// handle upload creation, locking, termination and so on. The composer is a
		// place where all those separated pieces are joined together. In this example
		// we only use the file store but you may plug in multiple.
		composer := tusd.NewStoreComposer()

		// let the composable storage tell tus which extensions it supports
		composable.UseIn(composer)

		config := tusd.Config{
			BasePath:      s.conf.Prefix,
			StoreComposer: composer,
			//Logger:        logger, // TODO use logger
		}

		handler, err := tusd.NewUnroutedHandler(config)
		if err != nil {
			return nil
		}
		return handler
	}
	return nil
}
