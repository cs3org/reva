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

package json

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"

	"github.com/cs3org/reva/pkg/meshdirectory"
	"github.com/cs3org/reva/pkg/meshdirectory/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

// New returns a new mesh directory manager object.
func New(m map[string]interface{}) (meshdirectory.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	// load mesh providers file
	model, err := load(c.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the providers")
		return nil, err
	}

	mgr := &mgr{
		c:     c,
		model: model,
	}

	return mgr, nil
}

func load(file string) (*meshDirectoryModel, error) {
	fd, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening the file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "error reading the data")
		return nil, err
	}

	m := &meshDirectoryModel{}
	if err := json.Unmarshal(data, m); err != nil {
		err = errors.Wrap(err, "error decoding data to json")
		return nil, err
	}
	m.file = file

	return m, nil
}

type meshDirectoryModel struct {
	file          string
	MeshProviders *[]meshdirectory.MeshProvider `json:"providers"`
}

type config struct {
	File string `mapstructure:"providers"`
}

type mgr struct {
	c          *config
	sync.Mutex // concurrent access to the file and loaded
	model      *meshDirectoryModel
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (m *mgr) GetMeshProviders() (*[]meshdirectory.MeshProvider, error) {
	return m.model.MeshProviders, nil
}
