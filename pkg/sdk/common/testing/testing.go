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
