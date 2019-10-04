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

package tusdsvc

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/tus/tusd/pkg/filestore"
	tusd "github.com/tus/tusd/pkg/handler"
)

func init() {
	httpserver.Register("tusdsvc", New)
}

type config struct {
	Prefix       string                            `mapstructure:"prefix"`
	Driver       string                            `mapstructure:"driver"`
	TmpFolder    string                            `mapstructure:"tmp_folder"`
	Drivers      map[string]map[string]interface{} `mapstructure:"drivers"`
	ProviderPath string                            `mapstructure:"provider_path"`
}

type svc struct {
	conf    *config
	handler http.Handler
	storage storage.FS
}

// New returns a new datasvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	if conf.TmpFolder == "" {
		conf.TmpFolder = os.TempDir()
	}

	if err := os.MkdirAll(conf.TmpFolder, 0755); err != nil {
		return nil, err
	}

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

func (s *svc) setHandler() (err error) {

	// Create a new FileStore instance which is responsible for
	// storing the uploaded file on disk in the specified directory.
	// This path _must_ exist before tusd will store uploads in it.
	// If you want to save them on a different medium, for example
	// a remote FTP server, you can implement your own storage backend
	// by implementing the tusd.DataStore interface.
	store := filestore.FileStore{
		Path: "/data/uploads",
	}

	// A storage backend for tusd may consist of multiple different parts which
	// handle upload creation, locking, termination and so on. The composer is a
	// place where all those separated pieces are joined together. In this example
	// we only use the file store but you may plug in multiple.
	composer := tusd.NewStoreComposer()
	// TODO use Terminater
	// TODO use Locker
	// TODO use Concater
	// TODO use LenghtDeferrer
	store.UseIn(composer)

	//logger := log.New(os.Stdout, "tusd ", log.Ldate|log.Ltime|log.Lshortfile)

	config := tusd.Config{
		BasePath:      "/tus/", //s.conf.Prefix,
		StoreComposer: composer,
		//Logger:        logger,
	}

	handler, err := tusd.NewUnroutedHandler(config)
	if err != nil {
		return err
	}

	s.handler = handler.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		log := appctx.GetLogger(r.Context())
		log.Info().Msgf("tusd routing: path=%s", r.URL.Path)
		switch r.Method {
		case "POST":
			handler.PostFile(w, r)
		case "HEAD":
			handler.HeadFile(w, r)
		case "PATCH":
			handler.PatchFile(w, r)
		case "GET":
			handler.GetFile(w, r)
		// TODO Only attach the DELETE handler if the Terminate() method is provided
		case "DELETE":
			handler.DelFile(w, r)
		}
	}))

	return err
}
