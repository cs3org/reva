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

package testing

import (
	"fmt"
	"strings"

	"github.com/cs3org/reva/pkg/sdk"
)

func formatTestMessage(funcName string, msg string, params ...interface{}) string {
	// Format parameter list
	paramList := make([]string, 0, len(params))
	for _, param := range params {
		paramList = append(paramList, fmt.Sprintf("%#v", param))
	}

	return fmt.Sprintf("%s(%s) -> %s", funcName, strings.Join(paramList, ", "), msg)
}

// FormatTestResult pretty-formats a function call along with its parameters, result and expected result.
func FormatTestResult(funcName string, wants interface{}, got interface{}, params ...interface{}) string {
	msg := fmt.Sprintf("Got: %#v; Wants: %#v", got, wants)
	return formatTestMessage(funcName, msg, params...)
}

// FormatTestError pretty-formats a function error.
func FormatTestError(funcName string, err error, params ...interface{}) string {
	msg := fmt.Sprintf("Error: %v", err)
	return formatTestMessage(funcName, msg, params...)
}

// CreateTestSession creates a Reva session for testing.
// For this, it performs a basic login using the specified credentials.
func CreateTestSession(host string, username string, password string) (*sdk.Session, error) {
	if session, err := sdk.NewSession(); err == nil {
		if err := session.Initiate(host, false); err == nil {
			if err := session.BasicLogin(username, password); err == nil {
				return session, nil
			}
		}
	}

	return nil, fmt.Errorf("unable to create the test session")
}
