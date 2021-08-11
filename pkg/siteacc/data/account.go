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
	"strings"
	"time"

	"github.com/cs3org/reva/pkg/siteacc/password"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/mentix/key"
	"github.com/cs3org/reva/pkg/utils"
)

// Account represents a single site account.
type Account struct {
	Email        string `json:"email"`
	Title        string `json:"title"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Organization string `json:"organization"`
	Website      string `json:"website"`
	Role         string `json:"role"`
	PhoneNumber  string `json:"phoneNumber"`

	Password password.Password `json:"password"`

	DateCreated  time.Time `json:"dateCreated"`
	DateModified time.Time `json:"dateModified"`

	Data AccountData `json:"data"`
}

// AccountData holds additional data for a site account.
type AccountData struct {
	APIKey      key.APIKey `json:"apiKey"`
	GOCDBAccess bool       `json:"gocdbAccess"`
	Authorized  bool       `json:"authorized"`
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

// Update copies the data of the given account to this account.
func (acc *Account) Update(other *Account, setPassword bool, copyData bool) error {
	if err := other.verify(false); err != nil {
		return errors.Wrap(err, "unable to update account data")
	}

	// Manually update fields
	acc.Title = other.Title
	acc.FirstName = other.FirstName
	acc.LastName = other.LastName
	acc.Organization = other.Organization
	acc.Website = other.Website
	acc.Role = other.Role
	acc.PhoneNumber = other.PhoneNumber

	if setPassword && other.Password.Value != "" {
		// If a password was provided, use that as the new one
		if err := acc.UpdatePassword(other.Password.Value); err != nil {
			return errors.Wrap(err, "unable to update account data")
		}
	}

	if copyData {
		acc.Data = other.Data
	}

	return nil
}

// UpdatePassword assigns a new password to the account, hashing it first.
func (acc *Account) UpdatePassword(pwd string) error {
	if err := acc.Password.Set(pwd); err != nil {
		return errors.Wrap(err, "unable to update the user password")
	}
	return nil
}

// Clone creates a copy of the account; if erasePassword is set to true, the password will be cleared in the cloned object.
func (acc *Account) Clone(erasePassword bool) *Account {
	clone := *acc

	if erasePassword {
		clone.Password.Clear()
	}

	return &clone
}

// CheckScopeAccess checks whether the user can access the specified scope.
func (acc *Account) CheckScopeAccess(scope string) bool {
	hasAccess := false

	switch strings.ToLower(scope) {
	case ScopeDefault:
		hasAccess = true

	case ScopeGOCDB:
		hasAccess = acc.Data.GOCDBAccess
	}

	return hasAccess
}

// Cleanup trims all string entries.
func (acc *Account) Cleanup() {
	acc.Email = strings.TrimSpace(acc.Email)
	acc.Title = strings.TrimSpace(acc.Title)
	acc.FirstName = strings.TrimSpace(acc.FirstName)
	acc.LastName = strings.TrimSpace(acc.LastName)
	acc.Organization = strings.TrimSpace(acc.Organization)
	acc.Website = strings.TrimSpace(acc.Website)
	acc.Role = strings.TrimSpace(acc.Role)
	acc.PhoneNumber = strings.TrimSpace(acc.PhoneNumber)
}

func (acc *Account) verify(verifyPassword bool) error {
	if acc.Email == "" {
		return errors.Errorf("no email address provided")
	} else if !utils.IsEmailValid(acc.Email) {
		return errors.Errorf("invalid email address: %v", acc.Email)
	}
	if acc.FirstName == "" || acc.LastName == "" {
		return errors.Errorf("no or incomplete name provided")
	}
	if acc.Organization == "" {
		return errors.Errorf("no organization provided")
	}
	if acc.Website != "" && !utils.IsValidWebAddress(acc.Website) {
		return errors.Errorf("invalid website provided")
	}
	if acc.Role == "" {
		return errors.Errorf("no role provided")
	}
	if acc.PhoneNumber != "" && !utils.IsValidPhoneNumber(acc.PhoneNumber) {
		return errors.Errorf("invalid phone number provided")
	}

	if verifyPassword {
		if !acc.Password.IsValid() {
			return errors.Errorf("no valid password set")
		}
	}

	return nil
}

// NewAccount creates a new site account.
func NewAccount(email string, title, firstName, lastName string, organization, website string, role string, phoneNumber string, password string) (*Account, error) {
	t := time.Now()

	acc := &Account{
		Email:        email,
		Title:        title,
		FirstName:    firstName,
		LastName:     lastName,
		Organization: organization,
		Website:      website,
		Role:         role,
		PhoneNumber:  phoneNumber,
		DateCreated:  t,
		DateModified: t,
		Data: AccountData{
			APIKey:      "",
			GOCDBAccess: false,
			Authorized:  false,
		},
	}

	// Set the user password, which also makes sure that the given password is strong enough
	if err := acc.UpdatePassword(password); err != nil {
		return nil, err
	}

	if err := acc.verify(true); err != nil {
		return nil, err
	}

	return acc, nil
}
