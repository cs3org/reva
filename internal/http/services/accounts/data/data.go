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
	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/apikey"
	"github.com/cs3org/reva/pkg/utils"
)

// Account represents a single user account.
type Account struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`

	Data AccountData `json:"data"`
}

// AccountData holds additional data for a user account.
type AccountData struct {
	APIKey     apikey.APIKey `json:"apiKey"`
	Authorized bool          `json:"authorized"`
}

// Accounts holds an array of user accounts.
type Accounts = []*Account

// NewAccount creates a new user account.
func NewAccount(email string, firstName, lastName string) (*Account, error) {
	if email == "" {
		return nil, errors.Errorf("no email address provided")
	} else if !utils.IsEmailValid(email) {
		return nil, errors.Errorf("invalid email address: %v", email)
	}

	if firstName == "" || lastName == "" {
		return nil, errors.Errorf("no or incomplete name provided")
	}

	acc := &Account{
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Data: AccountData{
			APIKey:     "",
			Authorized: false,
		},
	}

	return acc, nil
}
