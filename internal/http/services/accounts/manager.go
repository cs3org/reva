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

package accounts

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/internal/http/services/accounts/config"
	"github.com/cs3org/reva/internal/http/services/accounts/data"
	"github.com/cs3org/reva/internal/http/services/accounts/panel"
	"github.com/cs3org/reva/pkg/apikey"
)

// Manager is responsible for all user account related tasks.
type Manager struct {
	conf *config.Configuration
	log  *zerolog.Logger

	accounts data.Accounts
	storage  data.Storage

	panel *panel.Panel

	mutex sync.RWMutex
}

func (mngr *Manager) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	mngr.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	mngr.log = log

	mngr.accounts = make(data.Accounts, 0, 32) // Reserve some space for accounts

	// Create the user accounts storage and read all stored data
	if storage, err := mngr.createStorage(conf.Storage.Driver); err == nil {
		mngr.storage = storage
		mngr.readAllAccounts()
	} else {
		return errors.Wrap(err, "unable to create accounts storage")
	}

	// Create the web interface panel
	if pnl, err := panel.NewPanel(conf, log); err == nil {
		mngr.panel = pnl
	} else {
		return errors.Wrap(err, "unable to create panel")
	}

	return nil
}

func (mngr *Manager) createStorage(driver string) (data.Storage, error) {
	if driver == "file" {
		return data.NewFileStorage(mngr.conf, mngr.log)
	}

	return nil, errors.Errorf("unknown storage driver %v", driver)
}

func (mngr *Manager) readAllAccounts() {
	if accounts, err := mngr.storage.ReadAll(); err == nil {
		mngr.accounts = *accounts
	} else {
		// Just warn when not being able to read accounts
		mngr.log.Warn().Err(err).Msg("error while reading accounts")
	}
}

func (mngr *Manager) writeAllAccounts() {
	if err := mngr.storage.WriteAll(&mngr.accounts); err != nil {
		// Just warn when not being able to write accounts
		mngr.log.Warn().Err(err).Msg("error while writing accounts")
	}
}

func (mngr *Manager) findAccountByEmail(email string) *data.Account {
	if email == "" {
		return nil
	}

	// Perform a case-insensitive search of the given email address
	for _, account := range mngr.accounts {
		if strings.EqualFold(account.Email, email) {
			return account
		}
	}
	return nil
}

func (mngr *Manager) findAccountByAPIKey(key apikey.APIKey) *data.Account {
	if key == "" {
		return nil
	}

	// Perform a case-sensitive search of the given API key
	for _, account := range mngr.accounts {
		if account.Data.APIKey == key {
			return account
		}
	}
	return nil
}

func (mngr *Manager) ShowPanel(w http.ResponseWriter) error {
	// The panel only shows the stored accounts and offers actions through links, so let it use cloned data
	accounts := mngr.CloneAccounts()
	return mngr.panel.Execute(w, &accounts)
}

// CreateAccount creates a new account; if an account with the same email address already exists, an error is returned.
func (mngr *Manager) CreateAccount(accountData *data.Account) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	// Accounts must be unique (identified by their email address)
	if mngr.findAccountByEmail(accountData.Email) != nil {
		return errors.Errorf("an account with the specified email address already exists")
	}

	if account, err := data.NewAccount(accountData.Email, accountData.FirstName, accountData.LastName); err == nil {
		mngr.accounts = append(mngr.accounts, account)
		mngr.storage.AccountAdded(account)
		mngr.writeAllAccounts()
	} else {
		return errors.Wrap(err, "error while creating account")
	}

	return nil
}

// UpdateAccount updates the account identified by the account email; if no such account exists, an error is returned.
func (mngr *Manager) UpdateAccount(accountData *data.Account, copyData bool) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	account := mngr.findAccountByEmail(accountData.Email)
	if account == nil {
		return errors.Errorf("no account with the specified email exists")
	}

	if err := account.Copy(accountData, copyData); err == nil {
		account.DateModified = time.Now()

		mngr.storage.AccountUpdated(account)
		mngr.writeAllAccounts()
	} else {
		return errors.Wrap(err, "error while updating account")
	}

	return nil
}

// AuthorizeAccount sets the authorization status of the account identified by the account email; if no such account exists, an error is returned.
func (mngr *Manager) AuthorizeAccount(accountData *data.Account, authorized bool) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	account := mngr.findAccountByEmail(accountData.Email)
	if account == nil {
		return errors.Errorf("no account with the specified email exists")
	}

	account.Data.Authorized = authorized

	mngr.storage.AccountUpdated(account)
	mngr.writeAllAccounts()

	return nil
}

// RemoveAccount removes the account identified by the account email; if no such account exists, an error is returned.
func (mngr *Manager) RemoveAccount(accountData *data.Account) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	for i, account := range mngr.accounts {
		if strings.EqualFold(account.Email, accountData.Email) {
			mngr.accounts = append(mngr.accounts[:i], mngr.accounts[i+1:]...)
			mngr.storage.AccountRemoved(account)
			mngr.writeAllAccounts()
			return nil
		}
	}

	return errors.Errorf("no account with the specified email exists")
}

// CloneAccounts retrieves all accounts currently stored by cloning the data, thus avoiding race conflicts and making outside modifications impossible.
func (mngr *Manager) CloneAccounts() data.Accounts {
	mngr.mutex.RLock()
	defer mngr.mutex.RUnlock()

	clone := make(data.Accounts, 0)

	// To avoid any "deep copy" packages, use gob en- and decoding instead
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	if err := enc.Encode(mngr.accounts); err == nil {
		if err := dec.Decode(&clone); err != nil {
			// In case of an error, create an empty data set
			clone = make(data.Accounts, 0)
		}
	}

	return clone
}

func newManager(conf *config.Configuration, log *zerolog.Logger) (*Manager, error) {
	mngr := &Manager{}
	if err := mngr.initialize(conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the accounts manager")
	}
	return mngr, nil
}
