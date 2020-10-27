/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

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
