// Copyright 2018-2021 CERN
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
	"encoding/gob"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	applications "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpv "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	scope "github.com/cs3org/reva/pkg/auth/scope"
	"github.com/jedib0t/go-pretty/table"
)

type AppTokenListOpts struct {
	Long              bool
	All               bool
	OnlyExpired       bool
	ApplicationFilter string
	Label             string
}

var appTokenListOpts *AppTokenListOpts = &AppTokenListOpts{}

func appTokensListCommand() *command {
	cmd := newCommand("token-list")
	cmd.Description = func() string { return "list all the application tokens" }
	cmd.Usage = func() string { return "Usage: token-list [-flags]" }

	cmd.BoolVar(&appTokenListOpts.Long, "long", false, "long listing")
	cmd.BoolVar(&appTokenListOpts.All, "all", false, "print all tokens, also the expired")
	cmd.BoolVar(&appTokenListOpts.OnlyExpired, "expired", false, "print only expired token")
	cmd.StringVar(&appTokenListOpts.ApplicationFilter, "sope", "", "filter by scope")
	cmd.StringVar(&appTokenListOpts.Label, "label", "", "filter by label name")

	cmd.ResetFlags = func() {
		s := reflect.ValueOf(appTokenListOpts).Elem()
		s.Set(reflect.Zero(s.Type()))
	}

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

		listPw := filter(listResponse.AppPasswords, appTokenListOpts)

		if len(w) == 0 {
			err = printTableAppPasswords(listPw, appTokenListOpts.Long)
			if err != nil {
				return err
			}
		} else {
			enc := gob.NewEncoder(w[0])
			if err := enc.Encode(listPw); err != nil {
				return err
			}
		}
		return nil
	}
	return cmd
}

func printTableAppPasswords(listPw []*applications.AppPassword, long bool) error {
	shortHeader := table.Row{"Token", "Label", "Scope", "Expiration"}
	longHeader := table.Row{"Token", "Scope", "Label", "Expiration", "Creation Time", "Last Used Time"}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)

	if long {
		t.AppendHeader(longHeader)
	} else {
		t.AppendHeader(shortHeader)
	}

	for _, pw := range listPw {
		scopeFormatted, err := prettyFormatScope(pw.TokenScope)
		if err != nil {
			return err
		}
		if long {
			t.AppendRow(table.Row{pw.Password, scopeFormatted, pw.Label, formatTime(pw.Expiration), formatTime(pw.Ctime), formatTime(pw.Utime)})
		} else {
			t.AppendRow(table.Row{pw.Password, pw.Label, scopeFormatted, formatTime(pw.Expiration)})
		}
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
	}
	return scopeFormatted.String(), nil
}

// Filter the list of app password, based on the option selected by the user
func filter(listPw []*applications.AppPassword, opts *AppTokenListOpts) (filtered []*applications.AppPassword) {
	var filters Filters

	if opts.OnlyExpired {
		filters = append(filters, &FilterByExpired{})
	} else {
		filters = append(filters, &FilterByNotExpired{})
	}
	if opts.ApplicationFilter != "" {
		filters = append(filters, &FilterByApplicationName{name: opts.ApplicationFilter})
	}
	if opts.Label != "" {
		filters = append(filters, &FilterByLabel{label: opts.Label})
	}
	if opts.All {
		// discard all the filters
		filters = []Filter{&FilterByNone{}}
	}

	for _, pw := range listPw {
		if filters.In(pw) {
			filtered = append(filtered, pw)
		}
	}
	return
}

type Filter interface {
	In(*applications.AppPassword) bool
}

type FilterByApplicationName struct {
	name string
}
type FilterByNone struct{}
type FilterByExpired struct{}
type FilterByNotExpired struct{}
type FilterByLabel struct {
	label string
}
type Filters []Filter

func (f *FilterByApplicationName) In(pw *applications.AppPassword) bool {
	for app := range pw.TokenScope {
		if app == f.name {
			return true
		}
	}
	return false
}

func (f *FilterByNone) In(pw *applications.AppPassword) bool {
	return true
}

func (f *FilterByExpired) In(pw *applications.AppPassword) bool {
	return pw.Expiration != nil && pw.Expiration.Seconds <= uint64(time.Now().Unix())
}

func (f *FilterByNotExpired) In(pw *applications.AppPassword) bool {
	return !(&FilterByExpired{}).In(pw)
}

func (f *FilterByLabel) In(pw *applications.AppPassword) bool {
	return f.label != "" && pw.Label == f.label
}

func (f Filters) In(pw *applications.AppPassword) bool {
	for _, filter := range f {
		if !filter.In(pw) {
			return false
		}
	}
	return true
}
