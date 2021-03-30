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

package siteacc

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/internal/http/services/siteacc/config"
	"github.com/cs3org/reva/internal/http/services/siteacc/data"
	"github.com/cs3org/reva/internal/http/services/siteacc/email"
	"github.com/cs3org/reva/internal/http/services/siteacc/panel"
	"github.com/cs3org/reva/internal/http/services/siteacc/registration"
	"github.com/cs3org/reva/internal/http/services/siteacc/sitereg"
	"github.com/cs3org/reva/pkg/mentix/key"
	"github.com/cs3org/reva/pkg/smtpclient"
)

const (
	// FindByEmail holds the string value of the corresponding search criterium.
	FindByEmail = "email"
	// FindByAPIKey holds the string value of the corresponding search criterium.
	FindByAPIKey = "apikey"
	// FindBySiteID holds the string value of the corresponding search criterium.
	FindBySiteID = "siteid"
)

// Manager is responsible for all site account related tasks.
type Manager struct {
	conf *config.Configuration
	log  *zerolog.Logger

	accounts data.Accounts
	storage  data.Storage

	panel            *panel.Panel
	registrationForm *registration.Form
	smtp             *smtpclient.SMTPCredentials

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

	// Create the site accounts storage and read all stored data
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

	// Create the web interface registrationForm
	if frm, err := registration.NewForm(conf, log); err == nil {
		mngr.registrationForm = frm
	} else {
		return errors.Wrap(err, "unable to create registrationForm")
	}

	// Create the SMTP client
	if conf.SMTP != nil {
		mngr.smtp = smtpclient.NewSMTPCredentials(conf.SMTP)
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

func (mngr *Manager) findAccount(by string, value string) (*data.Account, error) {
	if len(value) == 0 {
		return nil, errors.Errorf("no search value specified")
	}

	var account *data.Account
	switch strings.ToLower(by) {
	case FindByEmail:
		account = mngr.findAccountByPredicate(func(account *data.Account) bool { return strings.EqualFold(account.Email, value) })

	case FindByAPIKey:
		account = mngr.findAccountByPredicate(func(account *data.Account) bool { return account.Data.APIKey == value })

	case FindBySiteID:
		account = mngr.findAccountByPredicate(func(account *data.Account) bool { return account.GetSiteID() == value })

	default:
		return nil, errors.Errorf("invalid search type %v", by)
	}

	if account != nil {
		return account, nil
	}

	return nil, errors.Errorf("no user found matching the specified criteria")
}

func (mngr *Manager) findAccountByPredicate(predicate func(*data.Account) bool) *data.Account {
	for _, account := range mngr.accounts {
		if predicate(account) {
			return account
		}
	}
	return nil
}

// ShowPanel writes the panel HTTP output directly to the response writer.
func (mngr *Manager) ShowPanel(w http.ResponseWriter) error {
	// The panel only shows the stored accounts and offers actions through links, so let it use cloned data
	accounts := mngr.CloneAccounts()
	return mngr.panel.Execute(w, &accounts)
}

// ShowRegistrationForm writes the registration registrationForm HTTP output directly to the response writer.
func (mngr *Manager) ShowRegistrationForm(w http.ResponseWriter) error {
	return mngr.registrationForm.Execute(w)
}

// CreateAccount creates a new account; if an account with the same email address already exists, an error is returned.
func (mngr *Manager) CreateAccount(accountData *data.Account) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	// Accounts must be unique (identified by their email address)
	if account, _ := mngr.findAccount(FindByEmail, accountData.Email); account != nil {
		return errors.Errorf("an account with the specified email address already exists")
	}

	if account, err := data.NewAccount(accountData.Email, accountData.FirstName, accountData.LastName); err == nil {
		mngr.accounts = append(mngr.accounts, account)
		mngr.storage.AccountAdded(account)
		mngr.writeAllAccounts()

		_ = email.SendAccountCreated(account, []string{account.Email, mngr.conf.NotificationsMail}, mngr.smtp)
	} else {
		return errors.Wrap(err, "error while creating account")
	}

	return nil
}

// UpdateAccount updates the account identified by the account email; if no such account exists, an error is returned.
func (mngr *Manager) UpdateAccount(accountData *data.Account, copyData bool) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	account, err := mngr.findAccount(FindByEmail, accountData.Email)
	if err != nil {
		return errors.Wrap(err, "user to update not found")
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

// FindAccount is used to find an account by various criteria.
func (mngr *Manager) FindAccount(by string, value string) (*data.Account, error) {
	mngr.mutex.RLock()
	defer mngr.mutex.RUnlock()

	account, err := mngr.findAccount(by, value)
	if err != nil {
		return nil, err
	}

	// Clone the account to avoid external data changes
	clonedAccount := *account
	return &clonedAccount, nil
}

// AuthorizeAccount sets the authorization status of the account identified by the account email; if no such account exists, an error is returned.
func (mngr *Manager) AuthorizeAccount(accountData *data.Account, authorized bool) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	account, err := mngr.findAccount(FindByEmail, accountData.Email)
	if err != nil {
		return errors.Wrap(err, "no account with the specified email exists")
	}

	authorizedOld := account.Data.Authorized
	account.Data.Authorized = authorized

	mngr.storage.AccountUpdated(account)
	mngr.writeAllAccounts()

	if account.Data.Authorized && account.Data.Authorized != authorizedOld {
		_ = email.SendAccountAuthorized(account, []string{account.Email, mngr.conf.NotificationsMail}, mngr.smtp)
	}

	return nil
}

// AssignAPIKeyToAccount is used to assign a new API key to the account identified by the account email; if no such account exists, an error is returned.
func (mngr *Manager) AssignAPIKeyToAccount(accountData *data.Account, flags int) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	account, err := mngr.findAccount(FindByEmail, accountData.Email)
	if err != nil {
		return errors.Wrap(err, "no account with the specified email exists")
	}

	if len(account.Data.APIKey) > 0 {
		return errors.Errorf("the account already has an API key assigned")
	}

	for {
		apiKey, err := key.GenerateAPIKey(key.SaltFromEmail(account.Email), flags)
		if err != nil {
			return errors.Wrap(err, "error while generating API key")
		}

		// See if the key already exists (super extremely unlikely); if so, generate a new one and try again
		if acc, _ := mngr.findAccount(FindByAPIKey, apiKey); acc != nil {
			continue
		}

		account.Data.APIKey = apiKey
		break
	}

	mngr.storage.AccountUpdated(account)
	mngr.writeAllAccounts()

	_ = email.SendAPIKeyAssigned(account, []string{account.Email, mngr.conf.NotificationsMail}, mngr.smtp)

	return nil
}

// UnregisterAccountSite unregisters the site associated with the given account.
func (mngr *Manager) UnregisterAccountSite(accountData *data.Account) error {
	mngr.mutex.RLock()
	defer mngr.mutex.RUnlock()

	account, err := mngr.findAccount(FindByEmail, accountData.Email)
	if err != nil {
		return errors.Wrap(err, "no account with the specified email exists")
	}

	salt := key.SaltFromEmail(account.Email)
	siteID, err := key.CalculateSiteID(account.Data.APIKey, salt)
	if err != nil {
		return errors.Wrap(err, "unable to get site ID")
	}

	if err := sitereg.UnregisterSite(mngr.conf.SiteRegistration.URL, account.Data.APIKey, siteID, salt); err != nil {
		return errors.Wrap(err, "error while unregistering the site")
	}

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
