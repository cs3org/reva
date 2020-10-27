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

package sdk_test

import (
	"fmt"
	"testing"

	"github.com/cs3org/reva/pkg/sdk"
	testintl "github.com/cs3org/reva/pkg/sdk/common/testing"
)

func TestSession(t *testing.T) {
	tests := []struct {
		host        string
		username    string
		password    string
		shouldList  bool
		shouldLogin bool
	}{
		{"sciencemesh-test.uni-muenster.de:9600", "test", "testpass", true, true},
		{"sciencemesh.cernbox.cern.ch:443", "invalid", "invalid", true, false},
		{"google.de:443", "invalid", "invalid", false, false},
	}

	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			if session, err := sdk.NewSession(); err == nil {
				if err := session.Initiate(test.host, false); err == nil {
					if _, err := session.GetLoginMethods(); err != nil && test.shouldList {
						t.Errorf(testintl.FormatTestError("Session.GetLoginMethods", err))
					} else if err == nil && !test.shouldList {
						t.Errorf(testintl.FormatTestError("Session.GetLoginMethods", fmt.Errorf("listing of login methods with an invalid host succeeded")))
					}

					if err := session.BasicLogin(test.username, test.password); err == nil && test.shouldLogin {
						if !session.IsValid() {
							t.Errorf(testintl.FormatTestError("Session.BasicLogin", fmt.Errorf("logged in, but session is invalid"), test.username, test.password))
						}
						if session.Token() == "" {
							t.Errorf(testintl.FormatTestError("Session.BasicLogin", fmt.Errorf("logged in, but received no token"), test.username, test.password))
						}
					} else if err != nil && test.shouldLogin {
						t.Errorf(testintl.FormatTestError("Session.BasicLogin", err, test.username, test.password))
					} else if err == nil && !test.shouldLogin {
						t.Errorf(testintl.FormatTestError("Session.BasicLogin", fmt.Errorf("logging in with invalid credentials succeeded"), test.username, test.password))
					} else {
						if session.IsValid() {
							t.Errorf(testintl.FormatTestError("Session.BasicLogin", fmt.Errorf("not logged in, but session is valid"), test.username, test.password))
						}
					}
				} else {
					t.Errorf(testintl.FormatTestError("Session.Initiate", err, test.host, false))
				}
			} else {
				t.Errorf(testintl.FormatTestError("NewSession", err))
			}
		})
	}
}
