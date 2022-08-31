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

package file

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cs3org/reva/v2/pkg/publicshare/manager/json/persistence"
)

type file struct {
	path        string
	initialized bool
}

// New returns a new Cache instance
func New(path string) persistence.Persistence {
	return file{
		path: path,
	}
}

func (p file) InitDB() error {
	// attempt to create the db file
	var fi os.FileInfo
	var err error
	if fi, err = os.Stat(p.path); os.IsNotExist(err) {
		folder := filepath.Dir(p.path)
		if err := os.MkdirAll(folder, 0755); err != nil {
			return err
		}
		if _, err := os.Create(p.path); err != nil {
			return err
		}
	}

	if fi == nil || fi.Size() == 0 {
		err := ioutil.WriteFile(p.path, []byte("{}"), 0644)
		if err != nil {
			return err
		}
	}
	p.initialized = true
	return nil
}

func (p file) ReadDB() (map[string]interface{}, error) {
	if !p.initialized {
		return nil, fmt.Errorf("not initialized")
	}
	db := map[string]interface{}{}
	readBytes, err := ioutil.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(readBytes, &db); err != nil {
		return nil, err
	}
	return db, nil
}

func (p file) WriteDB(db map[string]interface{}) error {
	if !p.initialized {
		return fmt.Errorf("not initialized")
	}
	dbAsJSON, err := json.Marshal(db)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(p.path, dbAsJSON, 0644)
}
