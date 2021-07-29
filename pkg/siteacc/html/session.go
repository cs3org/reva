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

package html

import (
	"net/http"
	"strings"
	"time"

	"github.com/cs3org/reva/pkg/siteacc/data"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// Session stores all data associated with an HTML session.
type Session struct {
	ID            string
	MigrationID   string
	RemoteAddress string
	CreationTime  time.Time
	Timeout       time.Duration

	LoggedInUser *data.Account

	Data map[string]interface{}

	expirationTime time.Time
	halflifeTime   time.Time

	sessionCookieName string
}

func getRemoteAddress(r *http.Request) string {
	// Remove the port number from the remote address
	remoteAddress := ""
	if address := strings.Split(r.RemoteAddr, ":"); len(address) == 2 {
		remoteAddress = address[0]
	}
	return remoteAddress
}

// Save stores the session ID in a cookie using a response writer.
func (sess *Session) Save(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   sess.sessionCookieName,
		Value:  sess.ID,
		MaxAge: int(sess.Timeout / time.Second),
	})
}

// VerifyRequest checks whether the provided request matches the stored session.
func (sess *Session) VerifyRequest(r *http.Request, verifyRemoteAddress bool) error {
	cookie, err := r.Cookie(sess.sessionCookieName)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve client session ID")
	}
	if cookie.Value != sess.ID {
		return errors.Errorf("the session ID doesn't match")
	}

	if verifyRemoteAddress && sess.RemoteAddress != "" {
		if !strings.EqualFold(getRemoteAddress(r), sess.RemoteAddress) {
			return errors.Errorf("remote address has changed (%v != %v)", r.RemoteAddr, sess.RemoteAddress)
		}
	}

	return nil
}

// HalftimePassed checks whether the session has passed the first half of its lifetime.
func (sess *Session) HalftimePassed() bool {
	return time.Now().After(sess.halflifeTime)
}

// HasExpired checks whether the session has reached is timeout.
func (sess *Session) HasExpired() bool {
	return time.Now().After(sess.expirationTime)
}

// NewSession creates a new session, giving it a random ID.
func NewSession(name string, timeout time.Duration, r *http.Request) *Session {
	session := &Session{
		ID:                uuid.NewString(),
		MigrationID:       "",
		RemoteAddress:     getRemoteAddress(r),
		CreationTime:      time.Now(),
		Timeout:           timeout,
		Data:              make(map[string]interface{}, 10),
		expirationTime:    time.Now().Add(timeout),
		halflifeTime:      time.Now().Add(timeout / 2),
		sessionCookieName: name,
	}
	return session
}
