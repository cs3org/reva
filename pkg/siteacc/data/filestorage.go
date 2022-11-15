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
	"os"
	"path/filepath"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// FileStorage implements a filePath-based storage.
type FileStorage struct {
	Storage

	conf *config.Configuration
	log  *zerolog.Logger

	operatorsFilePath string
	accountsFilePath  string
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

	if conf.Storage.File.OperatorsFile == "" {
		return errors.Errorf("no operators file set in the configuration")
	}
	storage.operatorsFilePath = conf.Storage.File.OperatorsFile

	if conf.Storage.File.AccountsFile == "" {
		return errors.Errorf("no accounts file set in the configuration")
	}
	storage.accountsFilePath = conf.Storage.File.AccountsFile

	// Create the file directories if necessary
	_ = os.MkdirAll(filepath.Dir(storage.operatorsFilePath), 0755)
	_ = os.MkdirAll(filepath.Dir(storage.accountsFilePath), 0755)

	return nil
}

func (storage *FileStorage) readData(file string, obj interface{}) error {
	// Read the data from the specified file
	jsonData, err := os.ReadFile(file)
	if err != nil {
		return errors.Wrapf(err, "unable to read file %v", file)
	}

	if err := json.Unmarshal(jsonData, obj); err != nil {
		return errors.Wrapf(err, "invalid file %v", file)
	}

	return nil
}

// ReadOperators reads all stored operators into the given data object.
func (storage *FileStorage) ReadOperators() (*Operators, error) {
	operators := &Operators{}
	if err := storage.readData(storage.operatorsFilePath, operators); err != nil {
		return nil, errors.Wrap(err, "error reading operators")
	}
	return operators, nil
}

// ReadAccounts reads all stored accounts into the given data object.
func (storage *FileStorage) ReadAccounts() (*Accounts, error) {
	accounts := &Accounts{}
	if err := storage.readData(storage.accountsFilePath, accounts); err != nil {
		return nil, errors.Wrap(err, "error reading accounts")
	}
	return accounts, nil
}

func (storage *FileStorage) writeData(file string, obj interface{}) error {
	// Write the data to the specified file
	jsonData, _ := json.MarshalIndent(obj, "", "\t")
	if err := os.WriteFile(file, jsonData, 0755); err != nil {
		return errors.Wrapf(err, "unable to write file %v", file)
	}
	return nil
}

// WriteOperators writes all stored operators from the given data object.
func (storage *FileStorage) WriteOperators(ops *Operators) error {
	if err := storage.writeData(storage.operatorsFilePath, ops); err != nil {
		return errors.Wrap(err, "error writing operators")
	}
	return nil
}

// WriteAccounts writes all stored accounts from the given data object.
func (storage *FileStorage) WriteAccounts(accounts *Accounts) error {
	if err := storage.writeData(storage.accountsFilePath, accounts); err != nil {
		return errors.Wrap(err, "error writing accounts")
	}
	return nil
}

// OperatorAdded is called when a sites has been added.
func (storage *FileStorage) OperatorAdded(op *Operator) {
	// Simply skip this action; all data is saved solely in WriteSites
}

// OperatorUpdated is called when a sites has been updated.
func (storage *FileStorage) OperatorUpdated(op *Operator) {
	// Simply skip this action; all data is saved solely in WriteSites
}

// OperatorRemoved is called when a sites has been removed.
func (storage *FileStorage) OperatorRemoved(op *Operator) {
	// Simply skip this action; all data is saved solely in WriteSites
}

// AccountAdded is called when an account has been added.
func (storage *FileStorage) AccountAdded(account *Account) {
	// Simply skip this action; all data is saved solely in WriteAccounts
}

// AccountUpdated is called when an account has been updated.
func (storage *FileStorage) AccountUpdated(account *Account) {
	// Simply skip this action; all data is saved solely in WriteAccounts
}

// AccountRemoved is called when an account has been removed.
func (storage *FileStorage) AccountRemoved(account *Account) {
	// Simply skip this action; all data is saved solely in WriteAccounts
}

// NewFileStorage creates a new file storage.
func NewFileStorage(conf *config.Configuration, log *zerolog.Logger) (*FileStorage, error) {
	storage := &FileStorage{}
	if err := storage.initialize(conf, log); err != nil {
		return nil, errors.Wrap(err, "unable to initialize the file storage")
	}
	return storage, nil
}
