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
	"time"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/mentix/key"
	"github.com/cs3org/reva/pkg/utils"
)

// Account represents a single site account.
type Account struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`

	DateCreated  time.Time `json:"dateCreated"`
	DateModified time.Time `json:"dateModified"`

	Data AccountData `json:"data"`
}

// AccountData holds additional data for a site account.
type AccountData struct {
	APIKey     key.APIKey `json:"apiKey"`
	Authorized bool       `json:"authorized"`
}

// Accounts holds an array of site accounts.
type Accounts = []*Account

// GetSiteID returns the site ID (generated from the API key) for the given account.
func (acc *Account) GetSiteID() key.SiteIdentifier {
	if id, err := key.CalculateSiteID(acc.Data.APIKey, key.SaltFromEmail(acc.Email)); err == nil {
		return id
	}

	return ""
}

// Copy copies the data of the given account to this account; if copyData is true, the account data is copied as well.
func (acc *Account) Copy(other *Account, copyData bool) error {
	if err := other.verify(); err != nil {
		return errors.Wrap(err, "unable to update account data")
	}

	// Manually update fields
	acc.FirstName = other.FirstName
	acc.LastName = other.LastName

	if copyData {
		acc.Data = other.Data
	}

	return nil
}

func (acc *Account) verify() error {
	if acc.Email == "" {
		return errors.Errorf("no email address provided")
	} else if !utils.IsEmailValid(acc.Email) {
		return errors.Errorf("invalid email address: %v", acc.Email)
	}

	if acc.FirstName == "" || acc.LastName == "" {
		return errors.Errorf("no or incomplete name provided")
	}

	return nil
}

// NewAccount creates a new site account.
func NewAccount(email string, firstName, lastName string) (*Account, error) {
	t := time.Now()

	acc := &Account{
		Email:        email,
		FirstName:    firstName,
		LastName:     lastName,
		DateCreated:  t,
		DateModified: t,
		Data: AccountData{
			APIKey:     "",
			Authorized: false,
		},
	}

	if err := acc.verify(); err != nil {
		return nil, err
	}

	return acc, nil
}
