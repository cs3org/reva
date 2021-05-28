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

type appTokenListOpts struct {
	Long              bool
	All               bool
	OnlyExpired       bool
	ApplicationFilter string
	Label             string
}

var listOpts *appTokenListOpts = &appTokenListOpts{}

func appTokensListCommand() *command {
	cmd := newCommand("token-list")
	cmd.Description = func() string { return "list all the application tokens" }
	cmd.Usage = func() string { return "Usage: token-list [-flags]" }

	cmd.BoolVar(&listOpts.Long, "long", false, "long listing")
	cmd.BoolVar(&listOpts.All, "all", false, "print all tokens, also the expired")
	cmd.BoolVar(&listOpts.OnlyExpired, "expired", false, "print only expired token")
	cmd.StringVar(&listOpts.ApplicationFilter, "sope", "", "filter by scope")
	cmd.StringVar(&listOpts.Label, "label", "", "filter by label name")

	cmd.ResetFlags = func() {
		s := reflect.ValueOf(listOpts).Elem()
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

		listPw := filterAppPasswords(listResponse.AppPasswords, listOpts)

		if len(w) == 0 {
			err = printTableAppPasswords(listPw, listOpts.Long)
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
func filterAppPasswords(listPw []*applications.AppPassword, opts *appTokenListOpts) (filtered []*applications.AppPassword) {
	var filters filters

	if opts.OnlyExpired {
		filters = append(filters, &filterByExpired{})
	} else {
		filters = append(filters, &filterByNotExpired{})
	}
	if opts.ApplicationFilter != "" {
		filters = append(filters, &filterByApplicationName{name: opts.ApplicationFilter})
	}
	if opts.Label != "" {
		filters = append(filters, &filterByLabel{label: opts.Label})
	}
	if opts.All {
		// discard all the filters
		filters = []filter{&filterByNone{}}
	}

	for _, pw := range listPw {
		if filters.in(pw) {
			filtered = append(filtered, pw)
		}
	}
	return
}

type filter interface {
	in(*applications.AppPassword) bool
}

type filterByApplicationName struct {
	name string
}
type filterByNone struct{}
type filterByExpired struct{}
type filterByNotExpired struct{}
type filterByLabel struct {
	label string
}
type filters []filter

func (f *filterByApplicationName) in(pw *applications.AppPassword) bool {
	for app := range pw.TokenScope {
		if app == f.name {
			return true
		}
	}
	return false
}

func (f *filterByNone) in(pw *applications.AppPassword) bool {
	return true
}

func (f *filterByExpired) in(pw *applications.AppPassword) bool {
	return pw.Expiration != nil && pw.Expiration.Seconds <= uint64(time.Now().Unix())
}

func (f *filterByNotExpired) in(pw *applications.AppPassword) bool {
	return !(&filterByExpired{}).in(pw)
}

func (f *filterByLabel) in(pw *applications.AppPassword) bool {
	return f.label != "" && pw.Label == f.label
}

func (f filters) in(pw *applications.AppPassword) bool {
	for _, filter := range f {
		if !filter.in(pw) {
			return false
		}
	}
	return true
}
