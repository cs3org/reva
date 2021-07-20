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

package manager

import (
	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// UsersManager is responsible for managing logged in users through session objects.
type UsersManager struct {
	conf *config.Configuration
	log  *zerolog.Logger

	accountsManager *AccountsManager
}

const (
	defaultPasswordLength = 12
)

func (mngr *UsersManager) initialize(conf *config.Configuration, log *zerolog.Logger, accountsManager *AccountsManager) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	mngr.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	mngr.log = log

	if accountsManager == nil {
		return errors.Errorf("no accounts manager provided")
	}
	mngr.accountsManager = accountsManager

	return nil
}

// LoginUser tries to login a given username/password pair. On success, the corresponding user account is stored in the session and a user token is returned.
func (mngr *UsersManager) LoginUser(name, password string, scope string, session *html.Session) (string, error) {
	account, err := mngr.accountsManager.FindAccountEx(FindByEmail, name, false)
	if err != nil {
		return "", errors.Wrap(err, "no account with the specified email exists")
	}

	// Verify the provided password
	if !account.Password.Compare(password) {
		return "", errors.Errorf("invalid password")
	}

	// Check if the user has access to the specified scope
	if !account.CheckScopeAccess(scope) {
		return "", errors.Errorf("no access to the specified scope granted")
	}

	// Store the user account in the session
	session.LoggedInUser = account

	// Encapsulate all necessary authentication information in a token
	token, err := generateUserToken(session)
	if err != nil {
		return "", errors.Wrap(err, "unable to generate user token")
	}

	return token, nil
}

// LogoutUser logs the current user out.
func (mngr *UsersManager) LogoutUser(session *html.Session) {
	// Just unset the user account stored in the session
	session.LoggedInUser = nil
}

// VerifyUserToken is used to verify a user token against the current session.
func (mngr *UsersManager) VerifyUserToken(token string, session *html.Session) (string, error) {
	if session.LoggedInUser == nil {
		return "", errors.Errorf("no user logged in")
	}

	utoken, err := extractUserToken(token, session)
	if err != nil {
		return "", errors.Wrap(err, "unable to verify user authentication")
	}

	// Check the token values against the session
	if utoken.SessionID != session.ID && utoken.SessionID != session.MigrationID {
		return "", errors.Errorf("session ID mismatch")
	}
	if utoken.User != session.LoggedInUser.Email {
		return "", errors.Errorf("session user mismatch")
	}

	// Refresh the user token, as the session ID might have changed due to migration
	newToken, err := generateUserToken(session)
	if err != nil {
		return "", errors.Wrap(err, "unable to generate user token")
	}

	return newToken, nil
}

// NewUsersManager creates a new users manager instance.
func NewUsersManager(conf *config.Configuration, log *zerolog.Logger, accountsManager *AccountsManager) (*UsersManager, error) {
	mngr := &UsersManager{}
	if err := mngr.initialize(conf, log, accountsManager); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the users manager")
	}
	return mngr, nil
}
