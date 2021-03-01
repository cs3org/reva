// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this filePath except in compliance with the License.
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

package data

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/internal/http/services/siteacc/config"
)

// FileStorage implements a filePath-based storage.
type FileStorage struct {
	Storage

	conf *config.Configuration
	log  *zerolog.Logger

	filePath string
}

func (storage *FileStorage) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	storage.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	storage.log = log

	if conf.Storage.File.File == "" {
		return errors.Errorf("no file set in the configuration")
	}
	storage.filePath = conf.Storage.File.File

	// Create the file directory if necessary
	dir := filepath.Dir(storage.filePath)
	_ = os.MkdirAll(dir, 0755)

	return nil
}

// ReadAll reads all stored accounts into the given data object.
func (storage *FileStorage) ReadAll() (*Accounts, error) {
	accounts := &Accounts{}

	// Read the data from the configured file
	jsonData, err := ioutil.ReadFile(storage.filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read file %v", storage.filePath)
	}

	if err := json.Unmarshal(jsonData, accounts); err != nil {
		return nil, errors.Wrapf(err, "invalid file %v", storage.filePath)
	}

	return accounts, nil
}

// WriteAll writes all stored accounts from the given data object.
func (storage *FileStorage) WriteAll(accounts *Accounts) error {
	// Write the data to the configured file
	jsonData, _ := json.MarshalIndent(accounts, "", "\t")
	if err := ioutil.WriteFile(storage.filePath, jsonData, 0755); err != nil {
		return errors.Wrapf(err, "unable to write file %v", storage.filePath)
	}

	return nil
}

// AccountAdded is called when an account has been added.
func (storage *FileStorage) AccountAdded(account *Account) {
	// Simply skip this action; all data is saved solely in WriteAll
}

// AccountUpdated is called when an account has been updated.
func (storage *FileStorage) AccountUpdated(account *Account) {
	// Simply skip this action; all data is saved solely in WriteAll
}

// AccountRemoved is called when an account has been removed.
func (storage *FileStorage) AccountRemoved(account *Account) {
	// Simply skip this action; all data is saved solely in WriteAll
}

// NewFileStorage creates a new filePath storage.
func NewFileStorage(conf *config.Configuration, log *zerolog.Logger) (*FileStorage, error) {
	storage := &FileStorage{}
	if err := storage.initialize(conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the filePath storage")
	}
	return storage, nil
}
