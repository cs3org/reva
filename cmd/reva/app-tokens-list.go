// Copyright 2018-2023 CERN
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

package main

import (
	"io"
	"os"
	"strings"
	"time"

	applications "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpv "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	scope "github.com/cs3org/reva/pkg/auth/scope"
	"github.com/jedib0t/go-pretty/table"
)

func appTokensListCommand() *command {
	cmd := newCommand("app-tokens-list")
	cmd.Description = func() string { return "list all the application tokens" }
	cmd.Usage = func() string { return "Usage: token-list" }

	cmd.Action = func(w ...io.Writer) error {
		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()
		listResponse, err := client.ListAppPasswords(ctx, &applications.ListAppPasswordsRequest{})

		if err != nil {
			return err
		}

		if listResponse.Status.Code != rpc.Code_CODE_OK {
			return formatError(listResponse.Status)
		}

		err = printTableAppPasswords(listResponse.AppPasswords)
		if err != nil {
			return err
		}

		return nil
	}
	return cmd
}

func printTableAppPasswords(listPw []*applications.AppPassword) error {
	header := table.Row{"Token", "Scope", "Label", "Expiration", "Creation Time", "Last Used Time"}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)

	t.AppendHeader(header)

	for _, pw := range listPw {
		scopeFormatted, err := prettyFormatScope(pw.TokenScope)
		if err != nil {
			return err
		}
		t.AppendRow(table.Row{pw.Password, scopeFormatted, pw.Label, formatTime(pw.Expiration), formatTime(pw.Ctime), formatTime(pw.Utime)})
	}

	t.Render()
	return nil
}

func formatTime(t *types.Timestamp) string {
	if t == nil {
		return ""
	}
	return time.Unix(int64(t.Seconds), 0).String()
}

func prettyFormatScope(scopeMap map[string]*authpv.Scope) (string, error) {
	var scopeFormatted strings.Builder
	for scType, sc := range scopeMap {
		scopeStr, err := scope.FormatScope(scType, sc)
		if err != nil {
			return "", err
		}
		scopeFormatted.WriteString(scopeStr)
		scopeFormatted.WriteString(", ")
	}
	return scopeFormatted.String()[:scopeFormatted.Len()-2], nil
}
